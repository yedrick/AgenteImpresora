package api

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"

	"collatech-agent/internal/config"
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

// TestLocalhostY127SonElMismoOrigen: para el navegador son origenes
// distintos, y esa diferencia dejaba una aplicacion bloqueada sin ninguna
// explicacion. Con "http://localhost:4200" permitido, la misma aplicacion
// servida en "http://127.0.0.1:4200" no podia leer ni /health.
func TestLocalhostY127SonElMismoOrigen(t *testing.T) {
	srv := newTestServer(t, func(c *config.Config) {
		c.AllowedCORS = []string{"http://localhost:4200"}
	})

	permitidos := []string{
		"http://localhost:4200",
		"http://127.0.0.1:4200",
		"http://[::1]:4200",
		"http://LOCALHOST:4200",
		"http://localhost:4200/",
	}
	for _, origen := range permitidos {
		t.Run("permite "+origen, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/health", nil)
			req.Header.Set("Origin", origen)
			req.RemoteAddr = "127.0.0.1:1234"
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "" {
				t.Errorf("sin cabecera de origen: el navegador bloquearia la respuesta")
			}
		})
	}

	// Lo que debe seguir bloqueado.
	for _, origen := range []string{
		"http://localhost:3000",        // otro puerto
		"http://micolinda.ejemplo.com", // otra maquina
		"null",                         // iframe con sandbox
		"http://localhost.atacante.com:4200",
	} {
		t.Run("bloquea "+origen, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/health", nil)
			req.Header.Set("Origin", origen)
			req.RemoteAddr = "127.0.0.1:1234"
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("se permitio %q, y no deberia (cabecera %q)", origen, got)
			}
		})
	}
}

// TestIPDeConfianzaEntraSinToken: una maquina nombrada en trusted_ips puede
// usar la API sin token, para convivir con un cliente ya desplegado que no
// sabe mandarlo. El resto de la red sigue necesitandolo, y las rutas de
// administracion siguen siendo solo para la propia PC del agente.
func TestIPDeConfianzaEntraSinToken(t *testing.T) {
	srv := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "el-token"
		c.TrustedIPs = []string{"192.168.1.20", "10.0.5.0/24"}
	})

	pedir := func(ruta, desde string) int {
		req := httptest.NewRequest("GET", ruta, nil)
		req.RemoteAddr = desde + ":5555"
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("la IP nombrada entra", func(t *testing.T) {
		if got := pedir("/api/status", "192.168.1.20"); got != 200 {
			t.Errorf("HTTP %d: la IP de confianza deberia entrar sin token", got)
		}
	})
	t.Run("el rango tambien", func(t *testing.T) {
		if got := pedir("/api/status", "10.0.5.77"); got != 200 {
			t.Errorf("HTTP %d: la IP del rango de confianza deberia entrar", got)
		}
	})
	t.Run("el resto de la red sigue pidiendo token", func(t *testing.T) {
		if got := pedir("/api/status", "192.168.1.99"); got != 401 {
			t.Errorf("HTTP %d: una IP que no esta en la lista deberia dar 401", got)
		}
	})
	t.Run("administracion sigue siendo solo local", func(t *testing.T) {
		// Aunque la IP sea de confianza: poder imprimir no es poder
		// reconfigurar el agente ni leer sus registros.
		if got := pedir("/api/logs", "192.168.1.20"); got != 403 {
			t.Errorf("HTTP %d: /api/logs deberia seguir bloqueado fuera de la PC del agente", got)
		}
	})
	t.Run("imprimir si", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/print/text",
			strings.NewReader(`{"printer":"P","text":"hola"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "192.168.1.20:5555"
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != 202 && rec.Code != 200 {
			t.Errorf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
	})
}

// TestSinListaTodoSigueIgual: sin trusted_ips, nada cambia.
func TestSinListaTodoSigueIgual(t *testing.T) {
	srv := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "el-token"
	})
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.RemoteAddr = "192.168.1.20:5555"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("HTTP %d: sin lista de confianza deberia seguir pidiendo token", rec.Code)
	}
}

// TestNoSeOfrecenRedesVirtuales: las interfaces de Docker y de maquinas
// virtuales existen solo dentro de esta PC. Salian en la lista de
// "conectate desde otra PC a estas direcciones" y desde fuera no llevan a
// ningun sitio: el operador prueba 172.17.0.1, le da "connection refused" y
// cree que el agente esta roto.
func TestNoSeOfrecenRedesVirtuales(t *testing.T) {
	virtuales := []string{
		// Linux y macOS
		"docker0", "br-1a2b3c", "veth7f3a", "virbr0", "vmnet8", "vboxnet0", "tailscale0", "utun3",
		// Windows: nombres descriptivos, no prefijos
		"vEthernet (WSL)", "vEthernet (Default Switch)", "Hyper-V Virtual Ethernet Adapter",
		"VirtualBox Host-Only Network", "VMware Network Adapter VMnet1",
		"Npcap Loopback Adapter", "TAP-Windows Adapter V9", "ZeroTier One",
	}
	for _, n := range virtuales {
		if !esInterfazVirtual(n) {
			t.Errorf("%s deberia descartarse: no es alcanzable desde otra PC", n)
		}
	}
	reales := []string{
		"eth0", "enp3s0", "wlo1", "wlan0", "en0",
		// Como los nombra Windows de verdad
		"Ethernet", "Ethernet 2", "Wi-Fi", "Conexion de area local",
	}
	for _, n := range reales {
		if esInterfazVirtual(n) {
			t.Errorf("%s se descarto y es una interfaz de red de verdad", n)
		}
	}
}

// TestAccesoEnRedSeVeDesdeLaRed: /api/network devuelve las URLs para
// alcanzar al agente, que es justo lo que quiere ver quien abre el panel
// desde otra PC. Estaba marcado como administracion y respondia 403, asi
// que esa seccion mostraba un error en bruto.
func TestAccesoEnRedSeVeDesdeLaRed(t *testing.T) {
	srv := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "el-token"
	})

	req := httptest.NewRequest("GET", "/api/network", nil)
	req.Header.Set("Authorization", "Bearer el-token")
	req.RemoteAddr = "192.168.1.50:5555"
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}

	// Y sin token sigue sin verse.
	req2 := httptest.NewRequest("GET", "/api/network", nil)
	req2.RemoteAddr = "192.168.1.50:5555"
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != 401 {
		t.Errorf("HTTP %d sin token: deberia seguir pidiendolo", rec2.Code)
	}
}

// TestLoSensibleSigueSoloEnLocal deja por escrito donde esta la linea.
func TestLoSensibleSigueSoloEnLocal(t *testing.T) {
	srv := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "el-token"
		c.TrustedIPs = []string{"192.168.1.50"}
	})
	for _, ruta := range []string{"/api/diagnostico", "/api/logs", "/api/token", "/api/support-bundle"} {
		t.Run(ruta, func(t *testing.T) {
			req := httptest.NewRequest("GET", ruta, nil)
			req.Header.Set("Authorization", "Bearer el-token")
			req.RemoteAddr = "192.168.1.50:5555" // con token Y de confianza
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			if rec.Code != 403 {
				t.Errorf("HTTP %d: %s deberia verse solo en la PC del agente", rec.Code, ruta)
			}
		})
	}
}

// TestAltaDeOrigenesDesdeElPanel: poder autorizar un origen desde el panel
// evita editar config.json a mano, que es donde la gente se equivoca y
// luego no entiende por que su web sigue bloqueada.
func TestAltaDeOrigenesDesdeElPanel(t *testing.T) {
	srv := newTestServer(t, func(c *config.Config) {
		c.AllowedCORS = []string{"http://localhost:4200"}
	})

	guardar := func(cuerpo string, desde string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/cors", strings.NewReader(cuerpo))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = desde
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	t.Run("se guarda y se aplica al momento", func(t *testing.T) {
		rec := guardar(`["http://localhost:4200","https://mi-tienda.com"]`, "127.0.0.1:1234")
		if rec.Code != 200 {
			t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
		// Sin reiniciar nada, el origen nuevo ya vale.
		req := httptest.NewRequest("GET", "/health", nil)
		req.Header.Set("Origin", "https://mi-tienda.com")
		req.RemoteAddr = "127.0.0.1:1234"
		r2 := httptest.NewRecorder()
		srv.ServeHTTP(r2, req)
		if r2.Header().Get("Access-Control-Allow-Origin") != "https://mi-tienda.com" {
			t.Error("el origen recien autorizado no vale todavia: haria falta reiniciar, y eso se olvida")
		}
	})

	t.Run("no se puede autorizar desde la red", func(t *testing.T) {
		if rec := guardar(`["https://atacante.com"]`, "192.168.1.50:1234"); rec.Code != 403 {
			t.Errorf("HTTP %d: desde la red no deberia poder darse permiso a si mismo", rec.Code)
		}
	})

	t.Run("se rechaza el comodin", func(t *testing.T) {
		rec := guardar(`["*"]`, "127.0.0.1:1234")
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "cajon") {
			t.Errorf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("se avisa de lo mal escrito", func(t *testing.T) {
		for _, malo := range []string{`["mi-tienda.com"]`, `["https://mi-tienda.com/panel"]`, `["ftp://x.com"]`} {
			rec := guardar(malo, "127.0.0.1:1234")
			if rec.Code != 400 {
				t.Errorf("se acepto %s (HTTP %d): no coincidiria nunca y nadie sabria por que", malo, rec.Code)
			}
		}
	})
}

// TestElPanelEscapaLosOrigenesRechazados: los origenes rechazados los
// manda quien intenta conectar, asi que son texto de un desconocido. Se
// pintan en el panel, que se abre en la PC del agente y puede autorizarlos
// de un clic: si no se escapan, basta una peticion con HTML en la cabecera
// Origin para colar codigo en esa pagina.
func TestElPanelEscapaLosOrigenesRechazados(t *testing.T) {
	html, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)

	i := strings.Index(s, `corsRechazados").innerHTML`)
	if i < 0 {
		t.Fatal("no se encontro el pintado de origenes rechazados: cambio el panel")
	}
	bloque := s[i:min(i+600, len(s))]

	if !strings.Contains(bloque, "esc(o)") {
		t.Error("los origenes rechazados se pintan sin escapar")
	}
	// Los atributos tienen que ir entre comillas dobles, que es lo que esc
	// escapa; con comillas simples no protegeria.
	if strings.Contains(bloque, "data-permitir='") {
		t.Error("atributo con comilla simple: esc no escapa ese caracter")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
