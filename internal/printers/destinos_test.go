package printers

import "testing"

func TestComprobarDestinoDirecto(t *testing.T) {
	casos := []struct {
		goos, destino string
		vale          bool
	}{
		// Colas del sistema: siempre.
		{"linux", "EPSON Caja", true},
		{"linux", "printer://EPSON Caja", true},
		{"windows", "COMANDA", true},
		{"windows", "USB001", true}, // Get ya explica que no es un destino

		// Puertos de impresora.
		{"linux", "/dev/usb/lp0", true},
		{"linux", "device:///dev/usb/lp1", true},
		{"linux", "/dev/lp0", true},
		{"linux", "/dev/ttyUSB0", true},
		{"linux", "/dev/ttyACM3", true},
		{"linux", "/dev/ttyS0", true},
		{"linux", "com://ttyUSB0", true},
		{"linux", "bt://rfcomm0", true},
		{"linux", "bt:///dev/rfcomm0", true},
		{"darwin", "/dev/cu.usbserial-1420", true},
		{"windows", "COM3", true},
		{"windows", "com://com12", true},
		{"windows", `com://\\.\COM3`, true},
		{"windows", "bt://COM5", true},
		{"windows", "com://LPT1", true},

		// Lo que no es una impresora.
		{"linux", "/dev/sda", false},
		{"linux", "/dev/sda1", false},
		{"linux", "/dev/mem", false},
		{"linux", "/dev/null", false},
		{"linux", "/dev/usb/../sda", false},
		{"linux", "/dev/usb/lp0/../../sda", false},
		{"linux", "device:///etc/collatech/config.json", false},
		{"linux", "device://./storage/settings.json", false},
		{"linux", "device://", false},
		{"linux", "com://../etc/passwd", false},
		{"linux", "bt://sda", false},
		{"linux", "/dev/ttyUSB0x", false},
		{"windows", `device://C:\Windows\win.ini`, false},
		{"windows", `com://C:\archivo`, false},
		{"windows", `com://\\.\PhysicalDrive0`, false},

		// Red: cualquier equipo menos la propia PC.
		{"linux", "tcp://192.168.1.50:9100", true},
		{"linux", "tcp://192.168.1.50", true},
		{"linux", "192.168.1.50:9100", true},
		{"linux", "tcp://impresora-cocina:9100", true},
		{"linux", "tcp://[fe80::1]:9100", true},
		{"linux", "tcp://127.0.0.1:6379", false},
		{"linux", "tcp://127.8.8.8", false},
		{"linux", "127.0.0.1:6379", false},
		{"linux", "tcp://localhost", false},
		{"linux", "LOCALHOST:6379", false},
		{"linux", "tcp://algo.localhost:80", false},
		{"linux", "tcp://[::1]:80", false},
		{"linux", "tcp://[::ffff:127.0.0.1]:80", false},
		{"linux", "tcp://0.0.0.0:80", false},
		{"linux", "tcp://[::]:80", false},
	}
	for _, c := range casos {
		err := comprobarDestinoDirecto(c.destino, c.goos)
		if (err == nil) != c.vale {
			t.Errorf("%s %q: vale=%v, err=%v", c.goos, c.destino, c.vale, err)
		}
	}
}
