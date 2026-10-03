package printers

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDestinoRelativoCuentaDesdeLaCarpetaDeDatos: un destino como
// "device://./salida.bin" tiene que caer en la carpeta de datos.
//
// El agente hace Chdir al directorio del ejecutable al arrancar, asi que sin
// esto la ruta apuntaba ahi y el trabajo fallaba con "no such file or
// directory" nombrando un archivo que, desde donde mira quien lo configuro,
// si existe. Pasa justo al montar una impresora de prueba que escribe a un
// archivo, que es la unica forma de probar el agente sin impresora.
func TestDestinoRelativoCuentaDesdeLaCarpetaDeDatos(t *testing.T) {
	datos := t.TempDir()
	destino := filepath.Join(datos, "salida.bin")
	if err := os.WriteFile(destino, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// Se trabaja desde otro sitio, como hace el agente tras el Chdir.
	otro := t.TempDir()
	anterior, _ := os.Getwd()
	if err := os.Chdir(otro); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(anterior)

	m := NewManager(nil)
	m.SetBaseDir(datos)
	if err := m.Print("device://./salida.bin", []byte("HOLA")); err != nil {
		t.Fatalf("no se pudo imprimir al destino relativo: %v", err)
	}

	got, err := os.ReadFile(destino)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "HOLA" {
		t.Errorf("el archivo tiene %q y se esperaba \"HOLA\"", got)
	}
}

// TestDestinoAbsolutoNoSeToca: /dev/usb/lp0 y demas rutas absolutas se
// quedan como estan, aunque haya carpeta de datos.
func TestDestinoAbsolutoNoSeToca(t *testing.T) {
	m := NewManager(nil)
	m.SetBaseDir("/var/lib/collatech")

	absoluta := "/dev/usb/lp0"
	if runtime.GOOS == "windows" {
		absoluta = `C:\dev\lp0`
	}
	if got := m.resolver(absoluta); got != absoluta {
		t.Errorf("se toco una ruta absoluta: %q -> %q", absoluta, got)
	}
	if got := m.resolver(`\\.\COM3`); got != `\\.\COM3` {
		t.Errorf("se toco un puerto COM: %q", got)
	}
	// Sin carpeta de datos, nada cambia: es lo que hacen las pruebas viejas.
	vacio := NewManager(nil)
	if got := vacio.resolver("./x.bin"); got != "./x.bin" {
		t.Errorf("sin baseDir no deberia tocarse nada: %q", got)
	}
}
