package api

import (
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

// newBenchServer monta un agente real sobre carpetas temporales.
func newBenchServer(b *testing.B) http.Handler {
	b.Helper()
	dir := b.TempDir()
	prev, _ := os.Getwd()
	_ = os.Chdir(dir)
	b.Cleanup(func() { _ = os.Chdir(prev) })

	cfg := config.Default()
	paths := config.DefaultPaths(dir)
	logger, err := logs.NewJSONLoggerLevel(paths.Logs, "error")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { logger.Close() })
	// Alias "cocina" -> /dev/null, para medir tambien la resolucion de alias.
	_ = os.MkdirAll(paths.Storage, 0o755)
	_ = os.WriteFile(paths.Settings, []byte(`{"default_printer":"device:///dev/null","paper_width":576,"image_scale":80,"aliases":[{"name":"cocina","printer":"device:///dev/null"}]}`), 0o600)

	pm := printers.NewManager(logger)
	qm := queue.New(queue.Options{Printers: pm, Logger: logger, Workers: 1, MaxRetries: 1, StorageDir: paths.Storage})
	qm.Start()
	b.Cleanup(qm.Stop)
	return NewServer(cfg, paths, pm, qm, logger).Routes()
}

func benchPost(b *testing.B, h http.Handler, path, body string) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "127.0.0.1:5000"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			b.Fatalf("HTTP %d: %s", rec.Code, rec.Body.String())
		}
	}
}

// Ticket tipico de punto de venta: cabecera, tabla de articulos, total y QR.
func BenchmarkPrintTicket(b *testing.B) {
	h := newBenchServer(b)
	body := `{"printer":"device:///dev/null","title":"CAFETERIA","width":576,"cut":true,
	 "lines":[{"text":"Cliente: Ana Pena"},
	  {"type":"table","table":{"header":true,"border":true,
	   "columns":[{"text":"Producto","width":24},{"text":"Bs","width":10,"align":"right"}],
	   "rows":[["Producto","Bs"],["Cafe con leche","12.00"],["Empanada","9.50"],["Jugo","10.00"]]}},
	  {"text":"TOTAL: Bs 31.50","box":true,"align":"center"}],
	 "qr":"https://kollatek.com","barcode":"123456789"}`
	benchPost(b, h, "/api/print/ticket", body)
}

func BenchmarkPrintText(b *testing.B) {
	h := newBenchServer(b)
	benchPost(b, h, "/api/print/text", `{"printer":"device:///dev/null","text":"Pedido #1001\nMesa 4","cut":true}`)
}

// Resolver un alias leia storage/settings.json del disco en CADA impresion.
func BenchmarkPrintConAlias(b *testing.B) {
	h := newBenchServer(b)
	benchPost(b, h, "/api/print/text", `{"printer":"cocina","text":"Comanda","cut":true}`)
}

func BenchmarkPrintHTML(b *testing.B) {
	h := newBenchServer(b)
	benchPost(b, h, "/api/print/html", `{"printer":"device:///dev/null","width":576,"cut":true,
	 "html":"<!DOCTYPE html><html><body><h1>CAFETERIA</h1><p>Cliente: <b>Ana</b></p><table><tr><td>Cafe</td><td>12.00</td></tr></table></body></html>"}`)
}
