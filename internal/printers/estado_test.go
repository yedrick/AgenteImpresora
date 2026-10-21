package printers

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// impresoraDeRed simula una termica en el puerto 9100 que contesta a DLE EOT
// con los bytes indicados (indice 0 para n=1). Sin respuestas, acepta la
// conexion y calla, como las impresoras que no informan de su estado.
func impresoraDeRed(t *testing.T, respuestas ...byte) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var cmd [3]byte
				for {
					if _, err := c.Read(cmd[:]); err != nil {
						return
					}
					n := int(cmd[2])
					if cmd[0] != 0x10 || cmd[1] != 0x04 || n < 1 || n > len(respuestas) {
						continue
					}
					_, _ = c.Write([]byte{respuestas[n-1]})
				}
			}(conn)
		}
	}()
	return "tcp://" + ln.Addr().String()
}

func TestEstadoPorRed(t *testing.T) {
	m := NewManager(nil)
	casos := []struct {
		nombre     string
		respuestas []byte
		ready      bool
		paper      string
		tapa       bool
		error      string
	}{
		{"lista", []byte{0x16, 0x12, 0x12, 0x12}, true, PaperOK, false, ""},
		{"queda poco papel", []byte{0x16, 0x12, 0x12, 0x1e}, true, PaperLow, false, ""},
		{"sin papel", []byte{0x16, 0x32, 0x12, 0x72}, false, PaperOut, false, "sin papel"},
		{"tapa abierta", []byte{0x16, 0x16, 0x12, 0x12}, false, PaperOK, true, "tapa abierta"},
		{"cuchilla", []byte{0x1e, 0x52, 0x1a, 0x12}, false, PaperOK, false, "cuchilla"},
		{"no informa", nil, true, PaperUnknown, false, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			e := m.Estado(impresoraDeRed(t, c.respuestas...))
			if !e.Online {
				t.Fatalf("la impresora responde y salio fuera de linea: %+v", e)
			}
			if e.Ready != c.ready || e.Paper != c.paper || e.CoverOpen != c.tapa {
				t.Fatalf("ready=%v paper=%s tapa=%v, se esperaba ready=%v paper=%s tapa=%v (%+v)",
					e.Ready, e.Paper, e.CoverOpen, c.ready, c.paper, c.tapa, e)
			}
			if !strings.Contains(e.Error, c.error) {
				t.Fatalf("error %q, se esperaba que dijera %q", e.Error, c.error)
			}
		})
	}
}

// Una IP donde no hay nadie escuchando es una impresora fuera de linea, con
// un motivo que el operador entienda.
func TestEstadoPorRedApagada(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	e := NewManager(nil).Estado("tcp://" + addr)
	if e.Online || e.Ready {
		t.Fatalf("un puerto cerrado salio en linea: %+v", e)
	}
	if !strings.Contains(e.Error, "rechaza la conexion") {
		t.Fatalf("el error no explica el motivo: %q", e.Error)
	}
}

func TestEstadoDispositivo(t *testing.T) {
	m := NewManager(nil)
	dir := t.TempDir()

	if e := m.Estado("device://" + filepath.Join(dir, "lp9")); e.Online || !strings.Contains(e.Error, "no esta conectada") {
		t.Fatalf("un dispositivo que no existe salio en linea: %+v", e)
	}

	archivo := filepath.Join(dir, "rollo.bin")
	if err := os.WriteFile(archivo, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if e := m.Estado("device://" + archivo); !e.Ready {
		t.Fatalf("el archivo de pruebas tiene que salir listo: %+v", e)
	}
}

func TestInterpretarDLEIncompleto(t *testing.T) {
	impresora := byte(0x16)
	e := interpretarDLE(&impresora, nil, nil, nil).cerrar()
	if !e.Ready || e.Paper != PaperUnknown {
		t.Fatalf("con solo la primera respuesta: %+v", e)
	}
	fuera := byte(0x1e)
	if e := interpretarDLE(&fuera, nil, nil, nil).cerrar(); e.Ready || e.Error != "fuera de linea" {
		t.Fatalf("el bit de fuera de linea no se interpreto: %+v", e)
	}
}
