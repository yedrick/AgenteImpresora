package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
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
		docRequest: docRequest{Printer: "POS1"},
		Title:      "ACME",
		QR:         "https://ejemplo/factura/123",
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
	doc := func(w int) resolved {
		return resolved{Doc: escpos.DocOptions{PaperWidth: w}}
	}
	estrecho := buildNativeTemplate(templateRequest{Template: "recibo"}, doc(384)).Bytes()
	ancho := buildNativeTemplate(templateRequest{Template: "recibo"}, doc(576)).Bytes()
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

	escribir := func(nombre, destino string) {
		body := `{"default_printer":"POS1","paper_width":576,"image_scale":80,"printers":[{"name":"` +
			nombre + `","target":"` + destino + `"}]}`
		if err := os.WriteFile(paths.Settings, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	destinos := func(nombre string) []string {
		res, err := s.resolve(docRequest{Printer: nombre})
		if err != nil {
			t.Fatalf("resolve(%q): %v", nombre, err)
		}
		return res.Targets
	}

	escribir("cocina", "IMPRESORA-A")
	if got := destinos("cocina"); len(got) != 1 || got[0] != "IMPRESORA-A" {
		t.Fatalf("primera lectura: %v", got)
	}
	// Dos veces seguidas debe dar lo mismo (y la segunda sale de la cache).
	if got := destinos("cocina"); got[0] != "IMPRESORA-A" {
		t.Fatalf("segunda lectura: %v", got)
	}

	// Al cambiar el archivo, la cache tiene que invalidarse. Se fuerza una
	// fecha distinta porque el test escribe en el mismo instante.
	escribir("cocina", "IMPRESORA-B")
	futuro := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(paths.Settings, futuro, futuro); err != nil {
		t.Fatal(err)
	}
	if got := destinos("cocina"); len(got) != 1 || got[0] != "IMPRESORA-B" {
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
		Printers:   []settings.Printer{{Name: "caja", Target: "IMPRESORA-C"}},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := s.resolve(docRequest{Printer: "caja"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Targets) != 1 || res.Targets[0] != "IMPRESORA-C" {
		t.Fatalf("la cache no se refresco al guardar: %v", res.Targets)
	}
}

// --- Opciones de documento --------------------------------------------------

func TestCutAceptaBooleanoYNombre(t *testing.T) {
	cases := map[string]escpos.CutMode{
		`true`:       escpos.CutPartial,
		`false`:      escpos.CutNone,
		`"partial"`:  escpos.CutPartial,
		`"full"`:     escpos.CutFull,
		`"none"`:     escpos.CutNone,
		`"completo"`: escpos.CutFull,
		`"ninguno"`:  escpos.CutNone,
	}
	for raw, want := range cases {
		var c CutSetting
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			t.Fatalf("cut=%s: %v", raw, err)
		}
		if !c.Set || c.Mode != want {
			t.Fatalf("cut=%s -> %q, esperaba %q", raw, c.Mode, want)
		}
	}
	var c CutSetting
	if err := json.Unmarshal([]byte(`"loquesea"`), &c); err == nil {
		t.Fatal("un modo de corte invalido deberia dar error")
	}
}

// Cada impresora puede tener su propio ancho: una de 58 mm en cocina y una de
// 80 mm en caja, sin repetirlo en cada llamada.
func TestCadaImpresoraTieneSusAjustes(t *testing.T) {
	dir := t.TempDir()
	paths := config.DefaultPaths(dir)
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: config.Default(), paths: paths}
	if _, err := s.saveSettingsFile(settings.Settings{
		PaperWidth: 384,
		Printers: []settings.Printer{
			{Name: "cocina", Target: "COCINA-1", PaperWidth: 384, Cut: "none"},
			{Name: "caja", Target: "CAJA-1,CAJA-2", PaperWidth: 576, Cut: "full", UpsideDown: true},
		},
	}); err != nil {
		t.Fatal(err)
	}

	cocina, err := s.resolve(docRequest{Printer: "cocina"})
	if err != nil {
		t.Fatal(err)
	}
	if cocina.Doc.PaperWidth != 384 || cocina.Doc.Cut != escpos.CutNone {
		t.Fatalf("cocina: %+v", cocina.Doc)
	}
	if cocina.Doc.Columns() != 32 {
		t.Fatalf("cocina deberia tener 32 columnas, tiene %d", cocina.Doc.Columns())
	}

	caja, err := s.resolve(docRequest{Printer: "caja"})
	if err != nil {
		t.Fatal(err)
	}
	if caja.Doc.PaperWidth != 576 || caja.Doc.Cut != escpos.CutFull || !caja.Doc.UpsideDown {
		t.Fatalf("caja: %+v", caja.Doc)
	}
	if len(caja.Targets) != 2 {
		t.Fatalf("caja deberia encolar en dos destinos, tiene %v", caja.Targets)
	}

	// Lo que trae la peticion manda sobre el perfil.
	no := false
	sobre, err := s.resolve(docRequest{Printer: "caja", Width: 384, UpsideDown: &no})
	if err != nil {
		t.Fatal(err)
	}
	if sobre.Doc.PaperWidth != 384 || sobre.Doc.UpsideDown {
		t.Fatalf("la peticion deberia mandar sobre el perfil: %+v", sobre.Doc)
	}
}

// Una impresora que no esta dada de alta se usa tal cual: no hace falta
// configurar nada para imprimir.
func TestImpresoraNoConfiguradaFunciona(t *testing.T) {
	s := &Server{cfg: config.Default(), paths: config.DefaultPaths(t.TempDir())}
	res, err := s.resolve(docRequest{Printer: "EPSON TM-T20"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Targets) != 1 || res.Targets[0] != "EPSON TM-T20" {
		t.Fatalf("destinos: %v", res.Targets)
	}
}

func TestImpresionAlRevesPorLaAPI(t *testing.T) {
	h := newTestServer(t, nil)
	rec := post(t, h, "/api/print/text", `{"printer":"P","text":"hola","upside_down":true}`, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
}

// Los elementos nuevos del ticket no deben romper nada.
func TestElementosDelTicket(t *testing.T) {
	h := newTestServer(t, nil)
	body := `{"printer":"P","width":576,"compact":true,"cut":"partial","lines":[
	 {"text":"CABECERA","scale_w":2,"scale_h":2,"align":"center"},
	 {"type":"rule","rule":"="},
	 {"text":"Invertido","invert":true},
	 {"type":"table","table":{"header":true,"columns":[{"text":"A","width":10}],"rows":[["A"],["b"]]}},
	 {"type":"qr","qr":{"data":"https://kollatek.com","size":6,"ec":"H"}},
	 {"type":"barcode","barcode":{"data":"123456789","type":"ean13","height":60,"hri":"below"}},
	 {"type":"feed","feed":2},
	 {"text":"Fin","box":true,"align":"center"}]}`
	if rec := post(t, h, "/api/print/ticket", body, ""); rec.Code != http.StatusAccepted {
		t.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
	}
}

func TestElementosInvalidosDanError(t *testing.T) {
	h := newTestServer(t, nil)
	cases := []string{
		`{"printer":"P","lines":[{"type":"qr","qr":{"data":"` + strings.Repeat("x", 4000) + `"}}]}`,
		`{"printer":"P","lines":[{"type":"barcode","barcode":{"data":"` + strings.Repeat("1", 400) + `"}}]}`,
		`{"printer":"P","lines":[{"type":"image","image":"no-es-base64"}]}`,
	}
	for _, body := range cases {
		if rec := post(t, h, "/api/print/ticket", body, ""); rec.Code != http.StatusBadRequest {
			t.Fatalf("esperaba 400, obtuve %d para %s", rec.Code, body[:60])
		}
	}
	if rec := post(t, h, "/api/print/text", `{"text":"sin impresora"}`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("sin impresora deberia ser 400, obtuve %d", rec.Code)
	}
}

// Los alias antiguos se siguen leyendo y se convierten en impresoras.
func TestMigracionDeAliasAntiguos(t *testing.T) {
	dir := t.TempDir()
	paths := config.DefaultPaths(dir)
	if err := os.MkdirAll(filepath.Dir(paths.Settings), 0o755); err != nil {
		t.Fatal(err)
	}
	viejo := `{"default_printer":"POS1","paper_width":576,"image_scale":80,
	 "aliases":[{"name":"cocina","printer":"EPSON Cocina,EPSON Barra","description":"Pedidos"}]}`
	if err := os.WriteFile(paths.Settings, []byte(viejo), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{cfg: config.Default(), paths: paths}
	res, err := s.resolve(docRequest{Printer: "cocina"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Targets) != 2 {
		t.Fatalf("el alias antiguo deberia dar dos destinos, dio %v", res.Targets)
	}
}

// El paquete de soporte tiene que llevar lo necesario para diagnosticar, y
// nunca el token.
func TestPaqueteDeSoporte(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "token-muy-secreto"
	})
	// Un trabajo para que la cola tenga algo.
	post(t, h, "/api/print/text", `{"printer":"P","text":"hola"}`, "")

	req := httptest.NewRequest(http.MethodGet, "/api/support-bundle", nil)
	req.RemoteAddr = "127.0.0.1:5000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type %q", ct)
	}

	datos := rec.Body.Bytes()
	z, err := zip.NewReader(bytes.NewReader(datos), int64(len(datos)))
	if err != nil {
		t.Fatalf("el zip no es valido: %v", err)
	}
	nombres := map[string]bool{}
	for _, f := range z.File {
		nombres[f.Name] = true
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		contenido, _ := io.ReadAll(rc)
		rc.Close()
		if bytes.Contains(contenido, []byte("token-muy-secreto")) {
			t.Fatalf("%s lleva el token en claro", f.Name)
		}
	}
	for _, esperado := range []string{"diagnostico.json", "cola.json", "LEEME.txt", "settings.json"} {
		if !nombres[esperado] {
			t.Fatalf("falta %s en el paquete; hay %v", esperado, nombres)
		}
	}
}

// Es un endpoint de administracion: desde la red no debe responder.
func TestPaqueteDeSoporteSoloEnLocal(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "secreto"
	})
	if rec := get(t, h, "/api/support-bundle", "192.168.1.77:5000", "secreto"); rec.Code != http.StatusForbidden {
		t.Fatalf("esperaba 403, obtuve %d", rec.Code)
	}
}

// Un bucle mal escrito en el sistema del cliente no debe gastar el rollo
// entero.
func TestLimiteDeImpresiones(t *testing.T) {
	h := newTestServer(t, nil)
	body := `{"printer":"P","text":"x"}`
	var aceptadas, frenadas int
	for i := 0; i < burstPorIP+20; i++ {
		switch post(t, h, "/api/print/text", body, "").Code {
		case http.StatusAccepted:
			aceptadas++
		case http.StatusTooManyRequests:
			frenadas++
		}
	}
	if frenadas == 0 {
		t.Fatalf("no se freno ninguna de %d peticiones seguidas", burstPorIP+20)
	}
	if aceptadas < burstPorIP {
		t.Fatalf("solo se aceptaron %d, el margen son %d", aceptadas, burstPorIP)
	}
}

// Consultar el estado no se limita: el panel lo hace cada pocos segundos.
func TestLasConsultasNoSeLimitan(t *testing.T) {
	h := newTestServer(t, nil)
	for i := 0; i < burstPorIP+50; i++ {
		if rec := get(t, h, "/api/status", "", ""); rec.Code != http.StatusOK {
			t.Fatalf("peticion %d: HTTP %d", i, rec.Code)
		}
	}
}

func TestElLimiteSeRecupera(t *testing.T) {
	l := newRateLimiter(5, 10)
	ahora := time.Now()
	for i := 0; i < 5; i++ {
		if !l.allow("1.2.3.4", ahora) {
			t.Fatalf("la peticion %d deberia pasar", i)
		}
	}
	if l.allow("1.2.3.4", ahora) {
		t.Fatal("la sexta deberia frenarse")
	}
	// Otro equipo tiene su propio margen.
	if !l.allow("5.6.7.8", ahora) {
		t.Fatal("otro origen no deberia verse afectado")
	}
	// Un segundo despues hay 10 fichas mas.
	if !l.allow("1.2.3.4", ahora.Add(time.Second)) {
		t.Fatal("deberia recuperarse con el tiempo")
	}
}
