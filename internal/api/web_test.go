package api

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"

	"collatech-agent/internal/settings"
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
	for _, nombre := range []string{"index.html", "designer.html", "diagnostico.html", "sdk.html", "impresoras.html", "docs.html"} {
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

	for _, nombre := range []string{"index.html", "designer.html", "diagnostico.html", "sdk.html", "impresoras.html", "docs.html"} {
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

// TestGuardarImpresoraIncompletaAvisa: el guardado descarta las entradas sin
// destino o con el nombre repetido, para que un settings.json viejo no
// impida arrancar. Por la API eso era una perdida silenciosa: respondia
// 200 "impresoras guardadas" y la impresora no quedaba.
func TestGuardarImpresoraIncompletaAvisa(t *testing.T) {
	casos := []struct {
		nombre   string
		lista    []settings.Printer
		esperado string
	}{
		{"sin destino",
			[]settings.Printer{{Name: "caja", Target: ""}},
			"no tiene destino"},
		{"destino en blanco",
			[]settings.Printer{{Name: "caja", Target: "   "}},
			"no tiene destino"},
		{"sin nombre",
			[]settings.Printer{{Name: "", Target: "tcp://192.168.1.5:9100"}},
			"no tiene nombre"},
		{"nombre repetido",
			[]settings.Printer{
				{Name: "caja", Target: "tcp://192.168.1.5:9100"},
				{Name: "Caja", Target: "tcp://192.168.1.6:9100"},
			},
			"esta repetido"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := validarImpresoras(c.lista)
			if err == nil {
				t.Fatal("no dio error: la impresora se perderia sin avisar")
			}
			if !strings.Contains(err.Error(), c.esperado) {
				t.Errorf("el mensaje fue %q y no menciona %q", err, c.esperado)
			}
		})
	}

	// Lo correcto tiene que pasar.
	ok := []settings.Printer{
		{Name: "caja", Target: "tcp://192.168.1.5:9100"},
		{Name: "cocina", Target: "/dev/usb/lp0"},
	}
	if err := validarImpresoras(ok); err != nil {
		t.Errorf("rechazo una lista valida: %v", err)
	}
}

// TestLaPreviaMuestraElGiro: activar el giro en el disenador tiene que
// verse en la vista previa. Si solo lo aplicara la impresora, la previa
// mentiria y el giro solo se notaria con el papel en la mano, que es justo
// lo que el disenador existe para evitar.
func TestLaPreviaMuestraElGiro(t *testing.T) {
	cuerpo := func(girado bool) image.Image {
		t.Helper()
		body := fmt.Sprintf(`{"width":576,"scale":1,"upside_down":%v,
			"layout":{"padding":0,"rows":[{"cols":[{"weight":1,"items":[{"text":"ARRIBA","size":"xl"}]}]},
			{"cols":[{"weight":1,"items":[{"type":"space","height":60}]}]}]}}`, girado)
		req := httptest.NewRequest("POST", "/api/preview", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		newTestServer(t, nil).ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
		img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatalf("la previa no es un PNG: %v", err)
		}
		return img
	}

	// Donde esta la tinta: arriba o abajo del bloque.
	mitadConTinta := func(img image.Image) string {
		b := img.Bounds()
		arriba, abajo := 0, 0
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				r, _, _, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				if r>>8 < 128 {
					if y < b.Dy()/2 {
						arriba++
					} else {
						abajo++
					}
				}
			}
		}
		if arriba > abajo {
			return "arriba"
		}
		return "abajo"
	}

	normal := mitadConTinta(cuerpo(false))
	girado := mitadConTinta(cuerpo(true))
	if normal != "arriba" {
		t.Fatalf("sin girar el texto deberia estar arriba, y esta %s", normal)
	}
	if girado != "abajo" {
		t.Errorf("al girar el texto deberia quedar abajo, y esta %s: la previa no refleja el giro", girado)
	}
}
