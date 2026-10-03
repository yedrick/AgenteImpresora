//go:build !windows

package printers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// En Linux y macOS el spooler del sistema es CUPS. Se habla con el por sus
// herramientas de linea de comandos en vez de enlazar con libcups: evita una
// dependencia nativa y funciona igual en ambos sistemas.
const cupsTimeout = 20 * time.Second

type cupsPrinter struct {
	name string
}

func newSpoolerPrinter(name string) Printer { return &cupsPrinter{name: name} }

func (p *cupsPrinter) Connect() error {
	if _, err := exec.LookPath("lp"); err != nil {
		return fmt.Errorf("no se encontro CUPS (comando 'lp'): instala cups o usa tcp://host:9100")
	}
	return nil
}

func (p *cupsPrinter) Disconnect() error { return nil }

// Print manda los bytes como trabajo RAW. La cola de CUPS tiene que estar
// configurada como raw (sin filtro), que es lo habitual para una termica
// ESC/POS; con un driver de por medio, los bytes se reinterpretarian.
func (p *cupsPrinter) Print(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("no hay datos que enviar a %q", p.name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), cupsTimeout)
	defer cancel()

	cmd := enC(ctx, "lp", "-d", p.name, "-o", "raw", "-")
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("tiempo agotado enviando a %q", p.name)
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		// "The printer or class does not exist" no le dice nada a quien
		// configura: no sabe que es una "clase", y el nombre que escribio le
		// parece correcto. Pasa al teclearlo mal, y sobre todo al renombrar
		// una impresora dejando el nombre viejo en la aplicacion. Se le dice
		// que impresoras hay de verdad.
		if strings.Contains(strings.ToLower(msg), "does not exist") {
			var hay []string
			for _, info := range enumSpoolerPrinters() {
				hay = append(hay, info.Name)
			}
			if len(hay) > 0 {
				return fmt.Errorf("no existe ninguna impresora llamada %q en este sistema. Las que hay: %s",
					p.name, strings.Join(hay, ", "))
			}
			return fmt.Errorf("no existe ninguna impresora llamada %q, y este sistema no tiene ninguna cola dada de alta", p.name)
		}
		return fmt.Errorf("lp fallo para %q: %s", p.name, msg)
	}
	return nil
}

func (p *cupsPrinter) Status() error {
	online, detail := cupsStatus(p.name)
	if !online {
		return fmt.Errorf("impresora %q no disponible: %s", p.name, detail)
	}
	return nil
}

func cupsStatus(name string) (bool, string) {
	lines, err := runLpstat("-p", name)
	if err != nil || len(lines) == 0 {
		return false, "no encontrada"
	}
	online, detail := parseLpstatLine(lines[0])
	return online, detail
}

// enumSpoolerPrinters lista las colas de CUPS.
func enumSpoolerPrinters() []Info {
	lines, err := runLpstat("-p")
	if err != nil {
		return nil
	}
	var out []Info
	for _, line := range lines {
		name := lpstatName(line)
		if name == "" {
			continue
		}
		online, detail := parseLpstatLine(line)
		out = append(out, Info{Name: name, Type: "cups", Online: online, Status: detail})
	}
	return out
}

// enC prepara un comando de CUPS con el idioma forzado a C.
//
// Hace falta porque la salida de lpstat viene traducida: en un sistema en
// espanol, "printer TM-T88V is idle" sale como "la impresora TM-T88V esta
// inactiva". El filtro buscaba lineas que empezaran por "printer " y las
// descartaba todas, asi que el panel no listaba NINGUNA impresora de CUPS
// aunque estuviera instalada y funcionando. Comprobado con una Epson
// TM-T88V en un Ubuntu en espanol.
//
// LC_ALL manda sobre LANG y sobre las demas LC_*, asi que con esa basta.
func enC(ctx context.Context, nombre string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, nombre, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	return cmd
}

func runLpstat(args ...string) ([]string, error) {
	if _, err := exec.LookPath("lpstat"); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// lpstat devuelve codigo distinto de cero cuando no hay impresoras; eso
	// no es un fallo, asi que se mira la salida y no el codigo.
	out, _ := enC(ctx, "lpstat", args...).Output()
	var lines []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "printer ") {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

// lpstatName extrae el nombre de una linea "printer NOMBRE is idle. ...".
// Devuelve cadena vacia para cualquier otra linea de lpstat.
func lpstatName(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "printer" {
		return ""
	}
	return fields[1]
}

func parseLpstatLine(line string) (bool, string) {
	low := strings.ToLower(line)
	switch {
	case strings.Contains(low, "disabled"):
		return false, "deshabilitada"
	case strings.Contains(low, "is idle"):
		return true, "lista"
	case strings.Contains(low, "now printing"):
		return true, "imprimiendo"
	default:
		return true, ""
	}
}

// detectDevices busca impresoras USB y puertos serie conectados. Se listan los
// nodos de /dev que existen; no se abren, para no molestar a un dispositivo
// que este imprimiendo.
func detectDevices() []Info {
	patterns := []struct{ glob, typ string }{
		{"/dev/usb/lp*", "usb"},
		{"/dev/ttyUSB*", "serial"},
		{"/dev/ttyACM*", "serial"},
	}
	var out []Info
	for _, p := range patterns {
		matches, err := filepath.Glob(p.glob)
		if err != nil {
			continue
		}
		sort.Strings(matches)
		for _, path := range matches {
			info := Info{Name: path, Type: p.typ, Address: path, Online: true}
			if _, err := os.Stat(path); err != nil {
				info.Online, info.Status = false, "no accesible"
			}
			out = append(out, info)
		}
	}
	return out
}
