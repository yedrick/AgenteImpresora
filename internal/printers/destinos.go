package printers

import (
	"fmt"
	"net"
	"net/netip"
	"path"
	"regexp"
	"runtime"
	"strings"
)

// Un destino que llega escrito en una peticion de impresion, sin estar dado
// de alta, solo vale si es de verdad una impresora.
//
// Get resuelve cualquier cosa, y hace bien: el operador puede dar de alta lo
// que quiera desde el panel, incluido un archivo para probar sin impresora.
// Pero la peticion la escribe quien llama a la API, que no es el operador:
// con "device://" podia anadir bytes a cualquier archivo que el agente
// abriera —y como servicio corre como root, eso incluye /dev/sda y su propio
// config.json—, y con "tcp://" mandarlos a servicios que solo escuchan en la
// PC del agente.

var (
	// dispositivosUnix son los nodos donde aparece una impresora: USB, puerto
	// paralelo, serie, Bluetooth emparejado y los puertos serie de macOS.
	dispositivosUnix = regexp.MustCompile(`^/dev/(usb/lp\d+|lp\d+|tty(USB|ACM|S|AMA)\d+|rfcomm\d+|(cu|tty)\.[A-Za-z0-9._-]+)$`)
	// dispositivosWindows son los puertos serie y paralelo.
	dispositivosWindows = regexp.MustCompile(`(?i)^\\\\\.\\(COM|LPT)\d+$`)
)

// ComprobarDestinoDirecto dice si un destino sin dar de alta se puede usar
// tal cual. Las colas del sistema (el nombre de una impresora) siempre valen.
func (m *Manager) ComprobarDestinoDirecto(name string) error {
	return comprobarDestinoDirecto(name, runtime.GOOS)
}

func comprobarDestinoDirecto(name, goos string) error {
	name = strings.TrimSpace(name)
	lower := strings.ToLower(name)

	switch {
	case strings.HasPrefix(lower, "tcp://"):
		return comprobarRed(name, withDefaultPort(name[len("tcp://"):]))
	case strings.HasPrefix(lower, "printer://"):
		return nil
	case strings.HasPrefix(lower, "com://"):
		return comprobarDispositivo(name, rutaDePuerto(name[len("com://"):], goos), goos)
	case strings.HasPrefix(lower, "bt://"):
		return comprobarDispositivo(name, rutaDePuerto(name[len("bt://"):], goos), goos)
	case strings.HasPrefix(lower, "device://"):
		return comprobarDispositivo(name, strings.TrimSpace(name[len("device://"):]), goos)
	case comPortRe.MatchString(name):
		return comprobarDispositivo(name, rutaDePuerto(name, goos), goos)
	case strings.HasPrefix(name, "/dev/"):
		return comprobarDispositivo(name, name, goos)
	case looksLikeHostPort(name):
		return comprobarRed(name, name)
	}
	return nil
}

// rutaDePuerto es devicePath con el sistema por parametro, para poder probar
// las reglas de Windows desde cualquier maquina.
func rutaDePuerto(port, goos string) string {
	port = strings.TrimSpace(port)
	if strings.HasPrefix(port, `\\.\`) || strings.HasPrefix(port, "/dev/") {
		return port
	}
	if goos == "windows" {
		return `\\.\` + strings.ToUpper(port)
	}
	return "/dev/" + port
}

func comprobarDispositivo(destino, ruta, goos string) error {
	valido := false
	if goos == "windows" {
		valido = dispositivosWindows.MatchString(ruta)
	} else {
		// path.Clean deshace los "..": /dev/usb/../sda es /dev/sda.
		valido = path.Clean(ruta) == ruta && dispositivosUnix.MatchString(ruta)
	}
	if valido {
		return nil
	}
	return errDestino(destino, "no es un puerto de impresora")
}

func comprobarRed(destino, addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errDestino(destino, "apunta a la propia PC del agente")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		ip = ip.Unmap()
		if ip.IsLoopback() || ip.IsUnspecified() {
			return errDestino(destino, "apunta a la propia PC del agente")
		}
	}
	return nil
}

func errDestino(destino, motivo string) error {
	return fmt.Errorf("el destino %q %s y no se puede usar directamente en una peticion. "+
		"Si es una impresora, dala de alta en el panel del agente (pestana Impresoras, "+
		"en su propia PC) y usa su nombre", destino, motivo)
}
