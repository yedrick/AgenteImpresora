// Package printers resuelve un destino de impresion y le envia bytes ESC/POS.
//
// Hay tres tipos de destino, y funcionan en todos los sistemas salvo donde se
// indica:
//
//	spooler  El sistema de impresion del SO: winspool en Windows, CUPS en
//	         Linux y macOS. Es el destino por defecto.
//	tcp      Impresora de red por el puerto RAW (9100 por defecto).
//	device   Un dispositivo de caracteres: puerto serie o impresora USB.
package printers

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"collatech-agent/internal/logs"
)

const (
	// DefaultRawPort es el puerto estandar de impresion RAW sobre TCP.
	DefaultRawPort = 9100

	// dialTimeout acota el establecimiento de la conexion TCP.
	dialTimeout = 5 * time.Second

	// writeTimeout acota la escritura hacia la impresora. Sin el, una
	// impresora que acepta la conexion y deja de leer (papel atascado, buffer
	// lleno) bloquea al worker para siempre.
	writeTimeout = 15 * time.Second

	// listCacheTTL evita reconsultar el sistema de impresion en cada
	// GET /api/printers: enumerar cuesta una llamada por impresora, y
	// detectar puertos serie los abre momentaneamente.
	listCacheTTL = 15 * time.Second
)

// Printer es un destino ya resuelto, listo para recibir bytes.
type Printer interface {
	Connect() error
	Disconnect() error
	Print([]byte) error
	Status() error
}

// Info describe una impresora detectada en el sistema.
type Info struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address,omitempty"`
	Online  bool   `json:"online"`
	Status  string `json:"status,omitempty"`

	// Model es el modelo reconocido en el catalogo, si se reconoce. Sirve
	// para proponer el ancho de papel y el modo de corte sin que el operador
	// tenga que saberselos.
	Model      string `json:"model,omitempty"`
	ModelName  string `json:"model_name,omitempty"`
	PaperWidth int    `json:"paper_width,omitempty"`
}

type Manager struct {
	logger *logs.Logger

	listMu   sync.Mutex
	listAt   time.Time
	listCopy []Info

	// baseDir es contra que se resuelven los destinos con ruta relativa.
	baseDir string
}

func NewManager(logger *logs.Logger) *Manager {
	return &Manager{logger: logger}
}

// SetBaseDir fija la carpeta contra la que se resuelven los destinos
// relativos, que es la de datos.
//
// Hace falta porque el agente hace Chdir al directorio del ejecutable al
// arrancar, para que los gestores de servicios encuentren configs/ y
// storage/. Sin esto, un destino como "device://./salida.bin" apuntaba a la
// carpeta del binario y no a la del usuario: el trabajo fallaba con "no such
// file or directory" senalando una ruta que, desde donde mira quien lo
// configuro, si existe.
func (m *Manager) SetBaseDir(dir string) { m.baseDir = dir }

// resolver convierte un destino relativo en absoluto. Una ruta ya absoluta
// (/dev/usb/lp0, C:\...) se devuelve igual.
func (m *Manager) resolver(destino string) string {
	if destino == "" || m.baseDir == "" || filepath.IsAbs(destino) {
		return destino
	}
	// Un puerto COM de Windows no es una ruta de archivo.
	if strings.HasPrefix(destino, `\\`) {
		return destino
	}
	return filepath.Join(m.baseDir, destino)
}

// List enumera las impresoras disponibles, con el resultado cacheado.
func (m *Manager) List() []Info {
	m.listMu.Lock()
	defer m.listMu.Unlock()
	if m.listCopy != nil && time.Since(m.listAt) < listCacheTTL {
		return m.listCopy
	}
	out := append(enumSpoolerPrinters(), detectDevices()...)
	if out == nil {
		out = []Info{}
	}
	for i := range out {
		if m, ok := MatchModel(out[i].Name); ok {
			out[i].Model = m.ID
			out[i].ModelName = m.Brand + " " + m.Name
			out[i].PaperWidth = m.PaperWidth
		}
	}
	m.listCopy, m.listAt = out, time.Now()
	return out
}

// Invalidate olvida la lista cacheada. Se usa tras un fallo de impresion, por
// si la impresora se desconecto.
func (m *Manager) Invalidate() {
	m.listMu.Lock()
	m.listCopy = nil
	m.listMu.Unlock()
}

var (
	// comPortRe solo acepta COM seguido de digitos: con el prefijo "com" a
	// secas, una impresora llamada "COMANDA" acababa enrutada al puerto serie.
	comPortRe = regexp.MustCompile(`(?i)^com\d+$`)
	usbPortRe = regexp.MustCompile(`(?i)^usb\d+$`)
)

// Get resuelve el destino a partir del nombre. Los prefijos explicitos mandan
// sobre cualquier heuristica, para poder nombrar una impresora del sistema que
// se parezca a un puerto.
//
//	tcp://192.168.1.50:9100   red (puerto 9100 si se omite)
//	printer://EPSON Caja      spooler del sistema, forzado
//	com://COM3                puerto serie, forzado
//	bt://COM5  bt://rfcomm0   impresora Bluetooth ya emparejada
//	device:///dev/usb/lp0     dispositivo, forzado
//	COM3  /dev/ttyUSB0        puerto serie
//	EPSON Caja                spooler del sistema (por defecto)
func (m *Manager) Get(name string) (Printer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("falta el nombre de la impresora")
	}
	lower := strings.ToLower(name)

	switch {
	case strings.HasPrefix(lower, "tcp://"):
		return newTCPPrinter(withDefaultPort(name[len("tcp://"):])), nil
	case strings.HasPrefix(lower, "printer://"):
		return newSpoolerPrinter(name[len("printer://"):]), nil
	case strings.HasPrefix(lower, "com://"):
		return newDevicePrinter(devicePath(name[len("com://"):]), "com"), nil
	case strings.HasPrefix(lower, "bt://"):
		return newBluetoothPrinter(name[len("bt://"):])
	case strings.HasPrefix(lower, "device://"):
		return newDevicePrinter(m.resolver(name[len("device://"):]), "device"), nil
	case comPortRe.MatchString(name):
		return newDevicePrinter(devicePath(name), "com"), nil
	case strings.HasPrefix(name, "/dev/"):
		return newDevicePrinter(name, "device"), nil
	case usbPortRe.MatchString(name):
		// "USB001" es el nombre de un puerto del spooler de Windows, no una
		// ruta de dispositivo: abrirlo como archivo nunca funciono.
		return nil, fmt.Errorf("%q es un puerto del spooler, no un destino: usa el nombre de la impresora (o printer://NOMBRE)", name)
	case looksLikeHostPort(name):
		return newTCPPrinter(name), nil
	default:
		return newSpoolerPrinter(name), nil
	}
}

// Print resuelve el destino, le envia los bytes y cierra.
func (m *Manager) Print(name string, payload []byte) error {
	// Un trabajo sin bytes no es un exito: devolver nil haria que la cola lo
	// marcara como Completed sin que la impresora recibiera nada.
	if len(payload) == 0 {
		return fmt.Errorf("trabajo vacio: no hay nada que enviar a %q", name)
	}
	p, err := m.Get(name)
	if err != nil {
		return err
	}
	if err := p.Connect(); err != nil {
		m.Invalidate()
		return err
	}
	defer p.Disconnect()
	if err := p.Print(payload); err != nil {
		m.Invalidate()
		return err
	}
	if m.logger != nil {
		m.logger.Info("print_sent", map[string]any{"printer": name, "bytes": len(payload)})
	}
	return nil
}

// devicePath convierte un nombre de puerto en la ruta del dispositivo.
func devicePath(port string) string {
	port = strings.TrimSpace(port)
	if strings.HasPrefix(port, `\\.\`) || strings.HasPrefix(port, "/dev/") {
		return port
	}
	if runtime.GOOS == "windows" {
		return `\\.\` + strings.ToUpper(port)
	}
	return filepath.Join("/dev", port)
}

func withDefaultPort(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return addr
	}
	if _, _, err := net.SplitHostPort(addr); err == nil {
		return addr
	}
	return net.JoinHostPort(addr, strconv.Itoa(DefaultRawPort))
}

// looksLikeHostPort exige un puerto numerico valido: con solo comprobar si
// contiene ":", una impresora llamada "HP LaserJet: Caja" se enrutaba a TCP.
func looksLikeHostPort(s string) bool {
	if strings.ContainsAny(s, ` \`) {
		return false
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" {
		return false
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

// newBluetoothPrinter resuelve una impresora Bluetooth ya emparejada.
//
// El emparejado lo hace el sistema operativo, no el agente: una vez
// emparejada, la impresora aparece como un puerto serie y se escribe en el
// igual que en cualquier otro. En Windows es un COM saliente; en Linux hay
// que crear el nodo con "rfcomm bind".
func newBluetoothPrinter(target string) (Printer, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("falta el puerto de la impresora Bluetooth, por ejemplo bt://COM5 o bt://rfcomm0")
	}
	if looksLikeMAC(target) {
		return nil, fmt.Errorf("%q es una direccion Bluetooth, no un puerto: %s", target, pairingHint())
	}
	return newDevicePrinter(devicePath(target), "bluetooth"), nil
}

// looksLikeMAC reconoce una direccion del tipo AA:BB:CC:DD:EE:FF.
func looksLikeMAC(s string) bool {
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return false
		}
		for _, c := range p {
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}

func pairingHint() string {
	if runtime.GOOS == "windows" {
		return "emparejala en Configuracion de Windows, mira que puerto COM saliente le asigna y usa bt://COMn"
	}
	return "emparejala con bluetoothctl y crea el nodo con 'sudo rfcomm bind 0 AA:BB:CC:DD:EE:FF', luego usa bt://rfcomm0"
}

// --- TCP -------------------------------------------------------------------

type tcpPrinter struct {
	addr string
	conn net.Conn
}

func newTCPPrinter(addr string) *tcpPrinter { return &tcpPrinter{addr: addr} }

func (p *tcpPrinter) Connect() error {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	conn, err := net.DialTimeout("tcp", p.addr, dialTimeout)
	if err != nil {
		return err
	}
	p.conn = conn
	return nil
}

func (p *tcpPrinter) Disconnect() error {
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

func (p *tcpPrinter) Print(data []byte) error {
	if p.conn == nil {
		return fmt.Errorf("la impresora de red %s no esta conectada", p.addr)
	}
	if err := p.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	_, err := p.conn.Write(data)
	return err
}

func (p *tcpPrinter) Status() error {
	conn, err := net.DialTimeout("tcp", p.addr, dialTimeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

// --- Dispositivo de caracteres (serie, USB) --------------------------------

type devicePrinter struct {
	path string
	typ  string
	file *os.File
}

func newDevicePrinter(path, typ string) *devicePrinter {
	return &devicePrinter{path: path, typ: typ}
}

// banderasDe decide como abrir el destino.
//
// Un dispositivo de caracteres (/dev/usb/lp0, /dev/ttyUSB0, COM3) ignora la
// posicion del archivo, asi que O_WRONLY basta. Pero apuntar a un archivo
// normal es la unica forma de probar el agente sin impresora, y ahi O_WRONLY
// a secas escribe siempre desde el byte 0: cada ticket pisaba al anterior y,
// si el anterior era mas largo, quedaba la mezcla de los dos sin aviso. Con
// O_APPEND el archivo se comporta como el rollo de papel.
//
// A proposito no se anade O_CREATE: si el destino no existe conviene que
// falle, porque una ruta de dispositivo mal escrita se convertiria en un
// archivo cualquiera y pareceria que imprime cuando no sale nada.
func banderasDe(path string) int {
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		return os.O_WRONLY | os.O_APPEND
	}
	return os.O_WRONLY
}

func (p *devicePrinter) Connect() error {
	f, err := os.OpenFile(p.path, banderasDe(p.path), 0)
	if err != nil {
		return err
	}
	p.file = f
	return nil
}

func (p *devicePrinter) Disconnect() error {
	if p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	return err
}

func (p *devicePrinter) Print(data []byte) error {
	if p.file == nil {
		return fmt.Errorf("el dispositivo %s no esta abierto", p.path)
	}
	// Los dispositivos de caracteres no siempre admiten deadline; que falle
	// aqui no es motivo para abortar la impresion.
	_ = p.file.SetWriteDeadline(time.Now().Add(writeTimeout))
	_, err := p.file.Write(data)
	return err
}

func (p *devicePrinter) Status() error {
	f, err := os.OpenFile(p.path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}
