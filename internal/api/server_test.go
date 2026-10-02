package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"collatech-agent/internal/config"
	"collatech-agent/internal/escpos"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
	"collatech-agent/internal/settings"
)

func newTestServer(t *testing.T, tune func(*config.Config)) http.Handler {
	t.Helper()
	// t.Chdir pide Go 1.24; el modulo declara 1.22, asi que se hace a mano.
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	cfg := config.Default()
	if tune != nil {
		tune(&cfg)
	}
	logger, logErr := logs.NewJSONLogger("logs")
	if logErr != nil {
		t.Fatalf("logger: %v", logErr)
	}
	t.Cleanup(func() { logger.Close() })

	pm := printers.NewManager(logger)
	paths := config.DefaultPaths(dir)
	qm := queue.New(queue.Options{Printers: pm, Logger: logger, Workers: 1, MaxRetries: 1, StorageDir: paths.Storage})
	qm.Start()
	t.Cleanup(qm.Stop)

	return NewServer(cfg, paths, pm, qm, logger).Routes()
}

func post(t *testing.T, h http.Handler, path, body, remote string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if remote == "" {
		remote = "127.0.0.1:5000"
	}
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, h http.Handler, path, remote, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if remote == "" {
		remote = "127.0.0.1:5000"
	}
	req.RemoteAddr = remote
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Un ticket con "box" y margenes hacia strings.Repeat con cuenta negativa,
// porque los margenes se sumaban despues de truncar el texto.
func TestTicketConBoxYMargenesNoRevienta(t *testing.T) {
	h := newTestServer(t, nil)
	largo := strings.Repeat("X", 40)
	for _, body := range []string{
		`{"printer":"P","width":384,"lines":[{"text":"` + largo + `","box":true,"ml":1}]}`,
		`{"printer":"P","width":384,"lines":[{"text":"` + largo + `","box":true,"mr":1}]}`,
		`{"printer":"P","width":384,"border":true,"lines":[{"text":"` + largo + `","box":true,"ml":4,"mr":4}]}`,
		`{"printer":"P","width":576,"lines":[{"text":"corto","box":true,"ml":20,"mr":20}]}`,
	} {
		if rec := post(t, h, "/api/print/ticket", body, ""); rec.Code != http.StatusAccepted {
			t.Fatalf("esperaba 202, obtuve %d para %s", rec.Code, body)
		}
	}
}

// Una fila con mas celdas que columnas desbordaba colWidths.
func TestTablaConCeldasDeMasNoRevienta(t *testing.T) {
	h := newTestServer(t, nil)
	for _, body := range []string{
		`{"printer":"P","width":384,"lines":[{"type":"table","table":{"columns":[{"text":"A","width":10}],"rows":[["a","b","c"]]}}]}`,
		`{"printer":"P","width":384,"lines":[{"type":"table","table":{"columns":[{"text":"A","width":8},{"text":"B","width":8}],"rows":[["solo"]]}}]}`,
		`{"printer":"P","width":384,"lines":[{"type":"table","table":{"header":true,"border":true,"columns":[{"text":"A","width":4}],"rows":[["x","y"],["z"]]}}]}`,
	} {
		if rec := post(t, h, "/api/print/ticket", body, ""); rec.Code != http.StatusAccepted {
			t.Fatalf("esperaba 202, obtuve %d para %s", rec.Code, body)
		}
	}
}

// Si algo vuelve a reventar, debe salir un 500 con JSON y no un socket cortado.
func TestElPanicDevuelveJSON(t *testing.T) {
	s := &Server{cfg: config.Default(), paths: config.DefaultPaths(t.TempDir())}
	h := s.recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("esperaba 500, obtuve %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("respuesta inesperada: %s", rec.Body.String())
	}
}

func TestEndpointsDeAdministracionSoloEnLocal(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "secreto"
	})
	for _, path := range []string{"/api/diagnostico", "/api/logs", "/api/token"} {
		// Ni siquiera con el token correcto desde la red.
		if rec := get(t, h, path, "192.168.1.77:5000", "secreto"); rec.Code != http.StatusForbidden {
			t.Fatalf("%s desde la red: esperaba 403, obtuve %d", path, rec.Code)
		}
		if rec := get(t, h, path, "127.0.0.1:5000", ""); rec.Code != http.StatusOK {
			t.Fatalf("%s en local: esperaba 200, obtuve %d (%s)", path, rec.Code, rec.Body.String())
		}
	}
	// Escribir configuracion tampoco se permite desde la red.
	if rec := post(t, h, "/api/settings", `{"paper_width":576}`, "192.168.1.77:5000"); rec.Code != http.StatusForbidden {
		t.Fatalf("POST /api/settings desde la red: esperaba 403, obtuve %d", rec.Code)
	}
}

func TestImprimirDesdeLaRedExigeToken(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "secreto"
	})
	body := `{"printer":"P","text":"hola","cut":true}`

	req := httptest.NewRequest(http.MethodPost, "/api/print/text", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.77:5000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("sin token: esperaba 401, obtuve %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/print/text", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.77:5000"
	req.Header.Set("Authorization", "Bearer equivocado")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token incorrecto: esperaba 401, obtuve %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/print/text", strings.NewReader(body))
	req.RemoteAddr = "192.168.1.77:5000"
	req.Header.Set("Authorization", "Bearer secreto")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("token correcto: esperaba 202, obtuve %d (%s)", rec.Code, rec.Body.String())
	}
}

// Con HasPrefix, "http://localhost.atacante.com" pasaba el filtro.
func TestCORSNoAceptaOrigenPorPrefijo(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowedCORS = []string{"http://localhost"}
	})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost.atacante.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("se autorizo un origen que solo comparte prefijo: %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost" {
		t.Fatalf("el origen exacto deberia autorizarse, obtuve %q", got)
	}
}

// El informe que se manda a soporte no debe llevar el token.
func TestElDiagnosticoNoFiltraElToken(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "token-muy-secreto"
	})
	rec := get(t, h, "/api/diagnostico", "127.0.0.1:5000", "")
	if strings.Contains(rec.Body.String(), "token-muy-secreto") {
		t.Fatal("el informe de diagnostico incluye el token")
	}
}

func TestSanitizeTicketNoRegistraElContenido(t *testing.T) {
	req := ticketRequest{
		Printer: "POS1",
		Title:   "ACME",
		QR:      "https://ejemplo/factura/123",
		Lines: []ticketLine{
			{Text: "Nombre: Ana Perez"},
			{Text: "Total: Bs 120"},
		},
	}
	b, err := json.Marshal(sanitizeTicket(req))
	if err != nil {
		t.Fatal(err)
	}
	for _, secreto := range []string{"Ana Perez", "Bs 120", "ACME", "ejemplo/factura"} {
		if strings.Contains(string(b), secreto) {
			t.Fatalf("el log incluye %q: %s", secreto, b)
		}
	}
	if !strings.Contains(string(b), `"lines":2`) {
		t.Fatalf("falta la forma del ticket: %s", b)
	}
}

// padBoth contaba bytes: una tilde desalineaba la columna del total.
func TestPadBothAlineaConAcentos(t *testing.T) {
	for _, c := range []struct{ left, right string }{
		{"TOTAL", "Bs 120.00"},
		{"TOTAL ARTICULOS", "Bs 120.00"},
		{"TOTAL ARTÍCULOS", "Bs 120.00"},
		{"COMISIÓN AÑO", "Bs 1.00"},
	} {
		got := padBoth(c.left, c.right, 32)
		if n := len([]rune(got)); n != 32 {
			t.Fatalf("%q + %q -> %d columnas, esperaba 32: %q", c.left, c.right, n, got)
		}
		if !strings.HasSuffix(got, c.right) {
			t.Fatalf("el importe deberia quedar pegado a la derecha: %q", got)
		}
	}
}

// Un recorte por bytes podia partir un caracter UTF-8 por la mitad.
func TestPadBothNoParteCaracteres(t *testing.T) {
	got := padBoth(strings.Repeat("ñ", 40), "Bs 1.00", 32)
	if !utf8.ValidString(got) {
		t.Fatalf("el recorte produjo UTF-8 invalido: %q", got)
	}
}

func TestSeAvisaDeCodigosDemasiadoLargos(t *testing.T) {
	h := newTestServer(t, nil)
	body := `{"printer":"P","barcode":"` + strings.Repeat("1", 300) + `"}`
	if rec := post(t, h, "/api/print/ticket", body, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("esperaba 400 por codigo de barras largo, obtuve %d", rec.Code)
	}
	ok := `{"printer":"P","barcode":"123456789"}`
	if rec := post(t, h, "/api/print/ticket", ok, ""); rec.Code != http.StatusAccepted {
		t.Fatalf("un codigo normal deberia aceptarse, obtuve %d", rec.Code)
	}
}

// Sin base64 el cuerpo es texto: debe salir en CP850, no en UTF-8.
func TestRawCodificaElTextoEnCP850(t *testing.T) {
	h := newTestServer(t, nil)
	if rec := post(t, h, "/api/print/raw", `{"printer":"P","data":"caña"}`, ""); rec.Code != http.StatusAccepted {
		t.Fatalf("esperaba 202, obtuve %d", rec.Code)
	}
	if got := escpos.EncodeCP850("caña"); len(got) != 4 || got[2] != 0xa4 {
		t.Fatalf("la codificacion CP850 no es la esperada: %#v", got)
	}
}

// El encadenado de strings.Contains hacia que "factura-qr" cayera en la rama
// "qr" porque se evaluaba antes.
func TestResolucionDelNombreDePlantilla(t *testing.T) {
	cases := map[string]string{
		"recibo":        "recibo",
		"recibo.html":   "recibo",
		"RECIBO.HTML":   "recibo",
		"comanda":       "comanda",
		"qr":            "qr",
		"factura":       "factura",
		"factura-qr":    "factura",
		"desconocida":   "factura",
		"":              "factura",
		"  texto.html ": "texto",
	}
	for in, want := range cases {
		if got := templateName(in); got != want {
			t.Fatalf("templateName(%q) = %q, esperaba %q", in, got, want)
		}
	}
}

// El endpoint debe listar las plantillas que el motor sabe construir, no
// archivos que nunca se renderizan.
func TestListaDePlantillasCoincideConElMotor(t *testing.T) {
	h := newTestServer(t, nil)
	rec := get(t, h, "/api/templates", "", "")
	var resp struct {
		Data []string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("no se listo ninguna plantilla")
	}
	for _, name := range resp.Data {
		if templateName(name) != name {
			t.Fatalf("la plantilla listada %q no la reconoce el motor", name)
		}
	}
}

// req.Width se ignoraba: todo salia a 32 columnas.
func TestLaPlantillaRespetaElAncho(t *testing.T) {
	estrecho := buildNativeTemplate(templateRequest{Template: "recibo", Width: 384}).Bytes()
	ancho := buildNativeTemplate(templateRequest{Template: "recibo", Width: 576}).Bytes()
	if len(ancho) <= len(estrecho) {
		t.Fatalf("un papel de 80 mm deberia producir lineas mas largas (%d vs %d bytes)", len(ancho), len(estrecho))
	}
}

// Las tres lineas del recuadro tienen que medir lo mismo: la de contenido
// salia un caracter mas ancha que los bordes.
func TestElRecuadroCuadra(t *testing.T) {
	for _, texto := range []string{"T", "TOTAL", "TOTAL: Bs 21.50", strings.Repeat("X", 60)} {
		for _, cols := range []int{32, 42, 48} {
			b := escpos.New()
			drawBoxLine(b, ticketLine{Text: texto, Align: "center"}, []rune(texto), cols, false)
			// Se cuenta en BYTES: la salida ya esta en CP850, donde cada
			// caracter ocupa uno. Contar runas interpretaria los bytes como
			// UTF-8 y, por ejemplo, 0xC4 0xBF se juntarian en una sola.
			var anchos []int
			for _, linea := range strings.Split(string(b.Bytes()), "\n") {
				if l := len(decodeBoxLine(linea)); l > 0 {
					anchos = append(anchos, l)
				}
			}
			if len(anchos) != 3 {
				t.Fatalf("cols=%d %q: esperaba 3 lineas, obtuve %d", cols, texto, len(anchos))
			}
			if anchos[0] != anchos[1] || anchos[1] != anchos[2] {
				t.Fatalf("cols=%d %q: lineas descuadradas %v", cols, texto, anchos)
			}
			if anchos[0] > cols {
				t.Fatalf("cols=%d %q: el recuadro mide %d, se sale del papel", cols, texto, anchos[0])
			}
		}
	}
}

// decodeBoxLine quita los comandos ESC/POS y deja los bytes CP850, que en una
// linea de marco equivalen uno a uno con los caracteres impresos.
func decodeBoxLine(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == 0x1b && i+2 < len(s):
			i += 2
		case s[i] == 0x1d && i+2 < len(s):
			i += 2
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// Los ajustes se cachean para no leer el disco en cada impresion, pero un
// cambio en el archivo tiene que verse sin reiniciar el agente.
func TestLaCacheDeAjustesSeRefresca(t *testing.T) {
	dir := t.TempDir()
	paths := config.DefaultPaths(dir)
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: config.Default(), paths: paths}

	escribir := func(alias, destino string) {
		body := `{"default_printer":"POS1","paper_width":576,"image_scale":80,"aliases":[{"name":"` +
			alias + `","printer":"` + destino + `"}]}`
		if err := os.WriteFile(paths.Settings, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	escribir("cocina", "IMPRESORA-A")
	if got, _ := s.resolvePrinters("cocina"); len(got) != 1 || got[0] != "IMPRESORA-A" {
		t.Fatalf("primera lectura: %v", got)
	}
	// Dos veces seguidas debe dar lo mismo (y la segunda sale de la cache).
	if got, _ := s.resolvePrinters("cocina"); got[0] != "IMPRESORA-A" {
		t.Fatalf("segunda lectura: %v", got)
	}

	// Al cambiar el archivo, la cache tiene que invalidarse. Se fuerza una
	// fecha distinta porque el test escribe en el mismo instante.
	escribir("cocina", "IMPRESORA-B")
	futuro := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(paths.Settings, futuro, futuro); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.resolvePrinters("cocina"); len(got) != 1 || got[0] != "IMPRESORA-B" {
		t.Fatalf("tras cambiar el archivo deberia leerse de nuevo, obtuve %v", got)
	}
}

// Guardar por la API tambien tiene que refrescar la cache.
func TestGuardarAjustesRefrescaLaCache(t *testing.T) {
	dir := t.TempDir()
	paths := config.DefaultPaths(dir)
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: config.Default(), paths: paths}

	if _, err := s.saveSettingsFile(settings.Settings{
		PaperWidth: 576,
		Aliases:    []settings.Alias{{Name: "caja", Printer: "IMPRESORA-C"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.resolvePrinters("caja"); len(got) != 1 || got[0] != "IMPRESORA-C" {
		t.Fatalf("la cache no se refresco al guardar: %v", got)
	}
}
