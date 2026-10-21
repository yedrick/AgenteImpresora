package printers

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

// Estado es lo que se sabe de una impresora sin imprimir nada.
//
// Hasta ahora la unica forma de saber si una impresora funcionaba era mandar
// un ticket y mirar si salia. Desde un backend eso no sirve: una impresora
// sin papel acepta el trabajo igual, y el sistema se entera cuando el cliente
// reclama.
type Estado struct {
	Target string `json:"target"`
	// Online: la impresora responde. No implica que pueda imprimir: puede
	// estar sin papel o con la tapa abierta.
	Online bool `json:"online"`
	// Ready: puede imprimir ahora mismo.
	Ready bool `json:"ready"`
	// Paper: ok, low (queda poco), out (sin papel) o unknown (la impresora
	// o la conexion no lo informan).
	Paper     string `json:"paper"`
	CoverOpen bool   `json:"cover_open,omitempty"`
	// Printing: tiene un trabajo en curso, asi que no se la ha consultado
	// para no interferir.
	Printing bool `json:"printing,omitempty"`
	// Error dice por que no esta lista, en palabras para el operador.
	Error string `json:"error,omitempty"`
	// Detail dice como se comprobo, para poder fiarse o no del resultado.
	Detail string `json:"detail,omitempty"`
}

const (
	PaperOK      = "ok"
	PaperLow     = "low"
	PaperOut     = "out"
	PaperUnknown = "unknown"
)

// sondeoTimeout acota cada consulta. Una impresora de red apagada no
// contesta al connect y, sin limite, una consulta de estado tardaria lo que
// el sistema operativo quiera.
const sondeoTimeout = 2 * time.Second

// cerrar calcula Ready a partir del resto, para no repetir la regla en cada
// tipo de conexion.
func (e Estado) cerrar() Estado {
	if e.Paper == "" {
		e.Paper = PaperUnknown
	}
	if e.Online && e.Error == "" {
		switch {
		case e.Paper == PaperOut:
			e.Error = "sin papel"
		case e.CoverOpen:
			e.Error = "tapa abierta"
		}
	}
	e.Ready = e.Online && e.Error == ""
	return e
}

// Estado consulta la impresora indicada, con el mismo formato de destino que
// al imprimir.
//
// Quien llama tiene que asegurarse de que no se esta imprimiendo en ella: en
// una impresora USB, abrir el dispositivo durante un trabajo falla, y en una
// de red la consulta se mezclaria con los datos del ticket.
func (m *Manager) Estado(target string) Estado {
	target = strings.TrimSpace(target)
	p, err := m.Get(target)
	if err != nil {
		return Estado{Target: target, Error: err.Error()}.cerrar()
	}
	var e Estado
	switch p := p.(type) {
	case *tcpPrinter:
		e = estadoTCP(p.addr)
	case *devicePrinter:
		e = estadoDispositivo(p.path)
	default:
		e = estadoSpooler(target)
	}
	e.Target = target
	return e.cerrar()
}

// --- Red -------------------------------------------------------------------

func estadoTCP(addr string) Estado {
	conn, err := net.DialTimeout("tcp", addr, sondeoTimeout)
	if err != nil {
		return Estado{Error: "no responde en " + addr + ": " + motivoRed(err)}
	}
	defer conn.Close()

	// DLE EOT n es la consulta de estado en tiempo real de ESC/POS. La
	// impresora contesta un byte por consulta, sin imprimir nada.
	var r [4]*byte
	for i := range r {
		b, ok := consultarDLE(conn, byte(i+1))
		if !ok {
			break
		}
		r[i] = &b
	}
	if r[0] == nil {
		return Estado{
			Online: true,
			Detail: "acepta conexiones en " + addr + " pero no informa de su estado",
		}
	}
	e := interpretarDLE(r[0], r[1], r[2], r[3])
	e.Detail = "consultada por red (DLE EOT)"
	return e
}

// motivoRed traduce los errores de red que se ven en la practica.
func motivoRed(err error) string {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "no contesta (apagada, desconectada o la IP cambio)"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "refused"):
		return "rechaza la conexion (el equipo existe pero no es una impresora en ese puerto)"
	case strings.Contains(msg, "no route"), strings.Contains(msg, "unreachable"):
		return "no hay ruta hasta ella (otra red o cable desconectado)"
	}
	return msg
}

func consultarDLE(conn net.Conn, n byte) (byte, bool) {
	_ = conn.SetDeadline(time.Now().Add(sondeoTimeout))
	if _, err := conn.Write([]byte{0x10, 0x04, n}); err != nil {
		return 0, false
	}
	var buf [1]byte
	if _, err := conn.Read(buf[:]); err != nil {
		return 0, false
	}
	// Todas las respuestas tienen fijos los bits 1 y 4 a uno y los bits 0 y
	// 7 a cero. Si no, lo que contesto no es un estado ESC/POS.
	if buf[0]&0x93 != 0x12 {
		return 0, false
	}
	return buf[0], true
}

// interpretarDLE traduce las respuestas a DLE EOT 1 a 4. Las que falten
// (nil) se dejan como desconocidas.
func interpretarDLE(impresora, offline, errores, papel *byte) Estado {
	e := Estado{Online: true, Paper: PaperUnknown}
	if impresora != nil && *impresora&0x08 != 0 {
		e.Error = "fuera de linea"
	}
	if offline != nil {
		b := *offline
		e.CoverOpen = b&0x04 != 0
		if b&0x20 != 0 {
			e.Paper = PaperOut
		}
		if b&0x40 != 0 && e.Error == "" {
			e.Error = "la impresora informa de un error"
		}
	}
	if errores != nil {
		b := *errores
		switch {
		case b&0x08 != 0:
			e.Error = "error de la cuchilla (papel atascado)"
		case b&0x20 != 0:
			e.Error = "error grave: apagala y vuelve a encenderla"
		case b&0x40 != 0:
			e.Error = "cabezal recalentado: espera un momento"
		}
	}
	if papel != nil {
		b := *papel
		switch {
		case b&0x60 != 0:
			e.Paper = PaperOut
		case b&0x0c != 0:
			e.Paper = PaperLow
		default:
			if e.Paper != PaperOut {
				e.Paper = PaperOK
			}
		}
	}
	return e
}

// --- Dispositivo -----------------------------------------------------------

func estadoDispositivo(path string) Estado {
	fi, err := os.Stat(path)
	if err != nil && !strings.HasPrefix(path, `\\.\`) {
		return Estado{Error: "no esta conectada: no existe " + path}
	}
	if err == nil && fi.Mode().IsRegular() {
		// Un archivo normal es la forma de probar sin impresora.
		return Estado{Online: true, Paper: PaperOK, Detail: "archivo de pruebas"}
	}
	return estadoDispositivoSO(path)
}

// estadoAbriendo es lo minimo que se puede saber de un puerto: si se abre.
func estadoAbriendo(path string) Estado {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return Estado{Error: fmt.Sprintf("no se puede abrir %s: %v", path, err)}
	}
	f.Close()
	return Estado{Online: true, Detail: "el puerto se abre; no informa de papel"}
}
