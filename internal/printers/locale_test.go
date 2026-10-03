package printers

import (
	"os"
	"strings"
	"testing"
)

// TestListaLasColasDeCUPS comprueba que las impresoras del sistema aparecen
// en la lista, sea cual sea el idioma del sistema.
//
// La salida de lpstat viene traducida: en un Ubuntu en espanol, "printer X
// is idle" sale como "la impresora X esta inactiva". El filtro buscaba
// lineas que empezaran por "printer " y las descartaba todas, asi que el
// panel no listaba NINGUNA impresora de CUPS aunque estuviera instalada,
// activada e imprimiendo. Se vio con una Epson TM-T88V en Ubuntu.
func TestListaLasColasDeCUPS(t *testing.T) {
	if _, err := os.Stat("/usr/bin/lpstat"); err != nil {
		t.Skip("no hay CUPS en esta maquina")
	}
	lineas, err := runLpstat("-p")
	if err != nil {
		t.Skipf("lpstat no se pudo ejecutar: %v", err)
	}
	if len(lineas) == 0 {
		t.Skip("esta maquina no tiene ninguna cola de CUPS dada de alta")
	}
	// Con el idioma forzado, toda linea de impresora empieza igual.
	for _, l := range lineas {
		if !strings.HasPrefix(l, "printer ") {
			t.Fatalf("linea sin normalizar: %q\nel idioma del sistema se esta colando en la salida", l)
		}
		if lpstatName(l) == "" {
			t.Errorf("no se saco el nombre de %q", l)
		}
	}
	t.Logf("%d colas de CUPS leidas correctamente", len(lineas))
}

// TestNombreYEstadoDeLpstat prueba el analisis con la salida real que da
// lpstat con LC_ALL=C, sin depender de que la maquina tenga impresoras.
func TestNombreYEstadoDeLpstat(t *testing.T) {
	casos := []struct {
		linea  string
		nombre string
		online bool
		estado string
	}{
		{"printer TM-T88V is idle.  enabled since Sat Oct  3 12:27:22 2026", "TM-T88V", true, "lista"},
		{"printer caja now printing caja-12.  enabled since Mon Jan  1 00:00:00 2026", "caja", true, "imprimiendo"},
		{"printer cocina disabled since Mon Jan  1 00:00:00 2026 -", "cocina", false, "deshabilitada"},
	}
	for _, c := range casos {
		if got := lpstatName(c.linea); got != c.nombre {
			t.Errorf("nombre %q, se esperaba %q", got, c.nombre)
		}
		online, estado := parseLpstatLine(c.linea)
		if online != c.online || estado != c.estado {
			t.Errorf("estado de %q: (%v, %q), se esperaba (%v, %q)", c.nombre, online, estado, c.online, c.estado)
		}
	}
}
