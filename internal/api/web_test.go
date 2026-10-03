package api

import (
	"regexp"
	"strings"
	"testing"
)

func TestSDKEmpaquetadoEnElBinario(t *testing.T) {
	b, err := webFS.ReadFile("web/sdk/collatech-sdk.tgz")
	if err != nil {
		t.Fatalf("el SDK no quedo dentro del binario: %v", err)
	}
	if len(b) < 10000 {
		t.Fatalf("el SDK incrustado mide %d bytes: parece truncado", len(b))
	}
	// Un .tgz empieza por la cabecera de gzip.
	if b[0] != 0x1f || b[1] != 0x8b {
		t.Fatalf("no parece un .tgz: empieza por %02x %02x", b[0], b[1])
	}
	t.Logf("SDK incrustado: %d bytes", len(b))
}

// TestPaginasUsanElTemaCompartido comprueba que las cuatro paginas enlazan
// la hoja compartida y que ninguna trae su propio modo oscuro automatico.
//
// Antes cada una tenia su paleta: el panel arrancaba claro, el disenador
// negro y el diagnostico no tenia modo oscuro. Al moverse entre paginas
// cambiaban los colores, y el selector del menu no mandaba sobre todas.
func TestPaginasUsanElTemaCompartido(t *testing.T) {
	for _, nombre := range []string{"index.html", "designer.html", "diagnostico.html", "sdk.html"} {
		t.Run(nombre, func(t *testing.T) {
			b, err := webFS.ReadFile("web/" + nombre)
			if err != nil {
				t.Fatal(err)
			}
			html := string(b)
			for _, exigido := range []string{`href="/tema.css"`, `src="/tema.js"`, `id="ct-menu"`} {
				if !strings.Contains(html, exigido) {
					t.Errorf("falta %s: la pagina no usaria el tema ni el menu compartidos", exigido)
				}
			}
			if strings.Contains(html, "prefers-color-scheme") {
				t.Error("trae su propio modo oscuro automatico: peleara con el selector del menu")
			}
		})
	}
}

// TestNingunaVariableDeColorSinDefinir: una var(--x) que no existe no da
// error, simplemente no pinta nada. Asi es como un boton se quedaba sin
// fondo al limpiar la paleta vieja, sin que nada avisara.
func TestNingunaVariableDeColorSinDefinir(t *testing.T) {
	css, err := webFS.ReadFile("web/tema.css")
	if err != nil {
		t.Fatal(err)
	}
	declara := regexp.MustCompile(`(--[a-z0-9-]+)\s*:`)
	usa := regexp.MustCompile(`var\((--[a-z0-9-]+)`)

	definidas := map[string]bool{}
	for _, m := range declara.FindAllStringSubmatch(string(css), -1) {
		definidas[m[1]] = true
	}

	for _, nombre := range []string{"index.html", "designer.html", "diagnostico.html", "sdk.html"} {
		b, err := webFS.ReadFile("web/" + nombre)
		if err != nil {
			t.Fatal(err)
		}
		html := string(b)
		propias := map[string]bool{}
		for _, m := range declara.FindAllStringSubmatch(html, -1) {
			propias[m[1]] = true
		}
		for _, m := range usa.FindAllStringSubmatch(html, -1) {
			if !definidas[m[1]] && !propias[m[1]] {
				t.Errorf("%s usa %s y nadie la define: ese color no se pintara", nombre, m[1])
			}
		}
	}
}
