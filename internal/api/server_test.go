package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
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
	qm := queue.NewManager(pm, logger, 1, 1)
	qm.Start()
	t.Cleanup(qm.Stop)

	return NewServer(cfg, pm, qm, logger).Routes()
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
	s := &Server{cfg: config.Default()}
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
