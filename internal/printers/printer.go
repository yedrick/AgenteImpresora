package printers

import (
	"fmt"
	"net"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"collatech-agent/internal/logs"
)

// DefaultRawPort es el puerto estandar de impresion RAW sobre TCP. Antes
// habia que escribirlo siempre a mano: "192.168.1.50" sin puerto no llegaba
// a la rama TCP y "tcp://192.168.1.50" fallaba con "missing port".
const DefaultRawPort = 9100

// writeTimeout acota la escritura hacia la impresora. Sin el, una impresora
// que acepta la conexion y deja de leer (papel atascado, buffer lleno)
// bloqueaba al worker para siempre y congelaba toda la cola.
const writeTimeout = 15 * time.Second

type Printer interface {
	Connect() error
	Disconnect() error
	Print([]byte) error
	Status() error
}

type Info struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address,omitempty"`
	Online  bool   `json:"online"`
	Status  string `json:"status,omitempty"`
}

type Manager struct {
	logger *logs.Logger

	serialMu   sync.Mutex
	serialAt   time.Time
	serialList []Info
}

func NewManager(logger *logs.Logger) *Manager {
	return &Manager{logger: logger}
}

func (m *Manager) List() []Info {
	var out []Info
	if runtime.GOOS == "windows" {
		out = append(out, enumWindowsPrinters()...)
	}
	out = append(out, m.detectSerialPorts()...)
	return out
}

// comPortRe solo acepta COM seguido de digitos. Antes bastaba el prefijo
// "com", asi que una impresora llamada "COMANDA" o "Comanda Cocina" acababa
// enrutada al puerto serie "\\.\COMANDA COCINA".
var (
	comPortRe = regexp.MustCompile(`(?i)^com\d+$`)
	usbPortRe = regexp.MustCompile(`(?i)^usb\d+$`)
)

// Get resuelve el destino. Los prefijos explicitos (tcp://, com://,
// printer://) mandan sobre cualquier heuristica, para poder nombrar una
// impresora de Windows que se parezca a un puerto.
func (m *Manager) Get(name string) (Printer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("printer is required")
	}
	lower := strings.ToLower(name)

	switch {
	case strings.HasPrefix(lower, "tcp://"):
		return NewTCPPrinter(withDefaultPort(name[len("tcp://"):]), 5*time.Second), nil
	case strings.HasPrefix(lower, "com://"):
		return NewFilePrinter(devicePath(name[len("com://"):]), "COM"), nil
	case strings.HasPrefix(lower, "printer://"):
		return NewWindowsPrinter(name[len("printer://"):]), nil
	case comPortRe.MatchString(name):
		return NewFilePrinter(devicePath(name), "COM"), nil
	case usbPortRe.MatchString(name):
		// "USB001" es el nombre de un puerto del spooler, no una ruta de
		// dispositivo: abrirlo como archivo nunca funciono. Se indica la via
		// que si funciona en lugar de fallar con "file not found".
		return nil, fmt.Errorf("%q es un puerto del spooler, no un destino directo: usa el nombre de la impresora en Windows (o printer://NOMBRE)", name)
	case looksLikeHostPort(name):
		return NewTCPPrinter(name, 5*time.Second), nil
	default:
		return NewWindowsPrinter(name), nil
	}
}

func devicePath(port string) string {
	port = strings.TrimSpace(port)
	if strings.HasPrefix(port, `\\.\`) {
		return port
	}
	return `\\.\` + strings.ToUpper(port)
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

// looksLikeHostPort exige un puerto numerico valido. Antes bastaba con que el
// nombre contuviera ":", asi que una impresora llamada "HP LaserJet: Caja" se
// enrutaba a TCP.
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

func (m *Manager) Print(name string, payload []byte) error {
	// Un trabajo sin bytes no es un exito: devolver nil aqui hacia que la cola
	// lo marcara como Completed sin que la impresora recibiera nada.
	if len(payload) == 0 {
		return fmt.Errorf("trabajo vacio: no hay nada que enviar a %q", name)
	}
	p, err := m.Get(name)
	if err != nil {
		return err
	}
	if err := p.Connect(); err != nil {
		return err
	}
	defer p.Disconnect()
	if err := p.Print(payload); err != nil {
		return err
	}
	if m.logger != nil {
		m.logger.Info("print_sent", map[string]any{"printer": name, "bytes": len(payload)})
	}
	return nil
}

// detectSerialPorts abre COM1..COM32 para ver cuales existen. Como eso toma
// el puerto momentaneamente, el resultado se cachea: antes cada
// GET /api/printers hacia 32 aperturas y podia molestar a una impresora serie
// que estuviera imprimiendo.
const serialCacheTTL = 30 * time.Second

func (m *Manager) detectSerialPorts() []Info {
	if runtime.GOOS != "windows" {
		return nil
	}
	m.serialMu.Lock()
	defer m.serialMu.Unlock()
	if time.Since(m.serialAt) < serialCacheTTL && m.serialList != nil {
		return m.serialList
	}
	var out []Info
	for i := 1; i <= 32; i++ {
		name := fmt.Sprintf("COM%d", i)
		path := devicePath(name)
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			_ = f.Close()
			out = append(out, Info{Name: name, Type: "com", Address: path, Online: true, Status: "disponible"})
		}
	}
	m.serialList, m.serialAt = out, time.Now()
	return out
}

type TCPPrinter struct {
	addr    string
	timeout time.Duration
	conn    net.Conn
}

func NewTCPPrinter(addr string, timeout time.Duration) *TCPPrinter {
	return &TCPPrinter{addr: addr, timeout: timeout}
}

func (p *TCPPrinter) Connect() error {
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
	conn, err := net.DialTimeout("tcp", p.addr, p.timeout)
	if err != nil {
		return err
	}
	p.conn = conn
	return nil
}

func (p *TCPPrinter) Disconnect() error {
	if p.conn == nil {
		return nil
	}
	err := p.conn.Close()
	p.conn = nil
	return err
}

func (p *TCPPrinter) Print(data []byte) error {
	if p.conn == nil {
		return fmt.Errorf("tcp printer is not connected")
	}
	if err := p.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	_, err := p.conn.Write(data)
	return err
}

func (p *TCPPrinter) Status() error {
	conn, err := net.DialTimeout("tcp", p.addr, p.timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

type FilePrinter struct {
	path string
	typ  string
	file *os.File
}

func NewFilePrinter(path, typ string) *FilePrinter {
	return &FilePrinter{path: path, typ: typ}
}

func (p *FilePrinter) Connect() error {
	f, err := os.OpenFile(p.path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	p.file = f
	return nil
}

func (p *FilePrinter) Disconnect() error {
	if p.file == nil {
		return nil
	}
	err := p.file.Close()
	p.file = nil
	return err
}

func (p *FilePrinter) Print(data []byte) error {
	if p.file == nil {
		return fmt.Errorf("%s printer is not connected", p.typ)
	}
	if err := p.file.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		// Los dispositivos serie no siempre soportan deadline; no es fatal.
		_ = err
	}
	_, err := p.file.Write(data)
	return err
}

func (p *FilePrinter) Status() error {
	f, err := os.OpenFile(p.path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}
