package printers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolucionDeDestinos(t *testing.T) {
	m := NewManager(nil)
	cases := []struct {
		name string
		want string // tipo concreto esperado
	}{
		{"tcp://192.168.1.50:9100", "*printers.tcpPrinter"},
		{"tcp://192.168.1.50", "*printers.tcpPrinter"}, // puerto 9100 por defecto
		{"192.168.1.50:9100", "*printers.tcpPrinter"},
		{"COM3", "*printers.devicePrinter"},
		{"com://COM3", "*printers.devicePrinter"},
		{"/dev/usb/lp0", "*printers.devicePrinter"},
		{"device:///dev/usb/lp0", "*printers.devicePrinter"},
		// Los que NO deben confundirse con un puerto ni con una direccion:
		{"COMANDA", "spooler"},
		{"Comanda Cocina", "spooler"},
		{"Comercial", "spooler"},
		{"HP LaserJet: Caja", "spooler"},
		{"EPSON TM-T20", "spooler"},
		{"printer://COM3", "spooler"},
	}
	for _, c := range cases {
		p, err := m.Get(c.name)
		if err != nil {
			t.Fatalf("%q: %v", c.name, err)
		}
		got := typeName(p)
		if c.want == "spooler" {
			if got == "*printers.tcpPrinter" || got == "*printers.devicePrinter" {
				t.Fatalf("%q deberia ir al spooler del sistema, fue a %s", c.name, got)
			}
			continue
		}
		if got != c.want {
			t.Fatalf("%q -> %s, esperaba %s", c.name, got, c.want)
		}
	}
}

func typeName(v any) string {
	switch v.(type) {
	case *tcpPrinter:
		return "*printers.tcpPrinter"
	case *devicePrinter:
		return "*printers.devicePrinter"
	default:
		return "spooler"
	}
}

func TestPuertoPorDefecto(t *testing.T) {
	cases := map[string]string{
		"192.168.1.50":      "192.168.1.50:9100",
		"192.168.1.50:9100": "192.168.1.50:9100",
		"192.168.1.50:515":  "192.168.1.50:515",
		"impresora":         "impresora:9100",
	}
	for in, want := range cases {
		if got := withDefaultPort(in); got != want {
			t.Fatalf("withDefaultPort(%q) = %q, esperaba %q", in, got, want)
		}
	}
}

// "USB001" es un puerto del spooler de Windows, no una ruta: abrirlo como
// archivo nunca funciono, asi que ahora se avisa en vez de fallar con
// "file not found".
func TestUSBDaUnMensajeUtil(t *testing.T) {
	_, err := NewManager(nil).Get("USB001")
	if err == nil {
		t.Fatal("deberia devolver un error explicativo")
	}
	if !contains(err.Error(), "printer://") {
		t.Fatalf("el mensaje deberia indicar la alternativa: %v", err)
	}
}

func TestNombreVacio(t *testing.T) {
	if _, err := NewManager(nil).Get("   "); err == nil {
		t.Fatal("un nombre vacio deberia ser error")
	}
}

// Un trabajo sin bytes no puede contar como impreso.
func TestTrabajoVacioEsError(t *testing.T) {
	if err := NewManager(nil).Print("POS1", nil); err == nil {
		t.Fatal("imprimir cero bytes deberia ser un error")
	}
}

// El destino de dispositivo funciona en cualquier sistema: se comprueba
// escribiendo sobre un archivo normal.
func TestImpresionADispositivo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("en Windows las rutas de dispositivo son \\\\.\\COMn")
	}
	path := filepath.Join(t.TempDir(), "impresora")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(nil).Print("device://"+path, []byte("HOLA")); err != nil {
		t.Fatalf("no se pudo imprimir: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "HOLA" {
		t.Fatalf("la impresora recibio %q", got)
	}
}

func TestListaSeCachea(t *testing.T) {
	m := NewManager(nil)
	first := m.List()
	second := m.List()
	if len(first) != len(second) {
		t.Fatal("la lista cacheada deberia ser estable")
	}
	m.Invalidate()
	if m.listCopy != nil {
		t.Fatal("Invalidate deberia vaciar la cache")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestDestinoBluetooth(t *testing.T) {
	m := NewManager(nil)
	// Un puerto ya emparejado se trata como dispositivo de caracteres.
	p, err := m.Get("bt://rfcomm0")
	if err != nil {
		t.Fatal(err)
	}
	if typeName(p) != "*printers.devicePrinter" {
		t.Fatalf("bt:// deberia resolverse como dispositivo, dio %s", typeName(p))
	}
	// Una direccion MAC no sirve: el emparejado lo hace el sistema.
	_, err = m.Get("bt://AA:BB:CC:DD:EE:FF")
	if err == nil {
		t.Fatal("una direccion MAC deberia devolver instrucciones")
	}
	if !contains(err.Error(), "rfcomm") && !contains(err.Error(), "COM") {
		t.Fatalf("el mensaje deberia explicar como emparejar: %v", err)
	}
	if _, err := m.Get("bt://"); err == nil {
		t.Fatal("bt:// sin puerto deberia ser error")
	}
}

func TestReconocerDireccionMAC(t *testing.T) {
	for _, s := range []string{"AA:BB:CC:DD:EE:FF", "00:11:22:33:44:55", "aa:bb:cc:dd:ee:ff"} {
		if !looksLikeMAC(s) {
			t.Fatalf("%q deberia reconocerse como MAC", s)
		}
	}
	for _, s := range []string{"COM5", "rfcomm0", "192.168.1.1:9100", "AA:BB:CC"} {
		if looksLikeMAC(s) {
			t.Fatalf("%q no es una MAC", s)
		}
	}
}
