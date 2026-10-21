//go:build linux

package printers

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// lpGetStatus es el ioctl LPGETSTATUS de linux/lp.h: devuelve el byte de
// estado del puerto de impresora, que usblp rellena preguntando a la
// impresora por USB.
const lpGetStatus = 0x060b

// Bits del byte de estado, tambien de linux/lp.h. ERROR y SELECT van en
// logica negativa: el bit a cero es lo que indica el problema.
const (
	lpPError  = 0x08
	lpPSelecd = 0x10
	lpPOutpa  = 0x20
)

func estadoDispositivoSO(path string) Estado {
	// O_NONBLOCK para que abrir nunca espere a la impresora.
	fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EBUSY) {
			// usblp solo admite un uso a la vez: la tiene otro programa,
			// casi siempre CUPS imprimiendo por su cuenta.
			return Estado{Online: true, Printing: true, Detail: "la esta usando otro programa"}
		}
		return Estado{Error: "no se puede abrir " + path + ": " + err.Error()}
	}
	defer unix.Close(fd)

	st, err := unix.IoctlGetInt(fd, lpGetStatus)
	if err != nil {
		// Un puerto serie no entiende LPGETSTATUS: se sabe que esta, no
		// como esta.
		return Estado{Online: true, Detail: "el puerto se abre; no informa de papel"}
	}
	e := interpretarLP(st)
	e.Detail = "consultada por USB (usblp)"
	return e
}

func interpretarLP(st int) Estado {
	e := Estado{Online: true, Paper: PaperOK}
	switch {
	case st&lpPOutpa != 0:
		e.Paper = PaperOut
	case st&lpPSelecd == 0:
		e.Error = "fuera de linea"
	case st&lpPError == 0:
		e.Error = "la impresora informa de un error (tapa abierta o papel atascado)"
	}
	return e
}

// usbConectado busca un dispositivo USB por su numero de serie. Devuelve si
// esta enchufado y, si usblp lo tiene, su nodo en /dev/usb.
func usbConectado(serial string) (bool, string) {
	dispositivos, _ := filepath.Glob("/sys/bus/usb/devices/*/serial")
	conectado := false
	for _, f := range dispositivos {
		if leerSys(f) == serial {
			conectado = true
			break
		}
	}
	if !conectado {
		return false, ""
	}
	nodos, _ := filepath.Glob("/sys/class/usbmisc/lp*")
	for _, n := range nodos {
		// El nodo cuelga de la interfaz; el serial es del dispositivo, un
		// nivel mas arriba. Hay que resolver el enlace antes de subir:
		// filepath.Join quita el ".." sin seguirlo y miraria otra carpeta.
		iface, err := filepath.EvalSymlinks(filepath.Join(n, "device"))
		if err != nil {
			continue
		}
		if leerSys(filepath.Join(filepath.Dir(iface), "serial")) == serial {
			return true, filepath.Join("/dev/usb", filepath.Base(n))
		}
	}
	return true, ""
}

func leerSys(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
