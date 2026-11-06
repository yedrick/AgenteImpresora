package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
)

// newTestServerAPI monta lo mismo que newTestServer pero devuelve el
// *Server (para poner Reabrir) y las rutas, con config.json ya escrito.
func newTestServerAPI(t *testing.T, tune func(*config.Config)) (*Server, config.Paths) {
	t.Helper()
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
	paths := config.DefaultPaths(dir)
	if err := config.Save(paths.Config, cfg); err != nil {
		t.Fatal(err)
	}
	logger, err := logs.NewJSONLogger("logs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger.Close() })
	pm := printers.NewManager(logger)
	qm := queue.New(queue.Options{Printers: pm, Logger: logger, Workers: 1, MaxRetries: 1, StorageDir: paths.Storage})
	qm.Start()
	t.Cleanup(qm.Stop)
	return NewServer(cfg, paths, pm, qm, logger), paths
}

// reabrirFalso anota los hosts que se le piden y falla si falla != nil.
type reabrirFalso struct {
	pedidos []string
	falla   error
}

func (f *reabrirFalso) Reabrir(host string) error {
	f.pedidos = append(f.pedidos, host)
	return f.falla
}

func leerRed(t *testing.T, h http.Handler) map[string]any {
	t.Helper()
	rec := get(t, h, "/api/red", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/red: %d %s", rec.Code, rec.Body.String())
	}
	var r struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r.Data
}

func TestActivarRedGuardaYAplica(t *testing.T) {
	s, paths := newTestServerAPI(t, nil)
	f := &reabrirFalso{}
	s.Reabrir = f.Reabrir
	h := s.Routes()

	if rec := post(t, h, "/api/red", `{"activo":true}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("activar: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.pedidos) != 1 || f.pedidos[0] != "0.0.0.0" {
		t.Fatalf("Reabrir tenia que recibir 0.0.0.0, recibio %v", f.pedidos)
	}
	cfg, err := config.Load(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "0.0.0.0" || !cfg.AllowRemote || cfg.AuthToken == "" {
		t.Fatalf("config guardada mal: host=%q remote=%v token vacio=%v", cfg.Host, cfg.AllowRemote, cfg.AuthToken == "")
	}
	if rec := get(t, h, "/api/status", "192.168.1.77:5000", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("desde la red sin token tiene que ser 401, es %d", rec.Code)
	}
	if rec := get(t, h, "/api/status", "192.168.1.77:5000", cfg.AuthToken); rec.Code != http.StatusOK {
		t.Fatalf("desde la red con el token nuevo tiene que ser 200, es %d", rec.Code)
	}
	if d := leerRed(t, h); d["activo"] != true {
		t.Fatalf("GET /api/red tiene que decir activo: %v", d)
	}
}

func TestDesactivarRedVuelveASoloEstaPC(t *testing.T) {
	s, paths := newTestServerAPI(t, func(c *config.Config) {
		c.Host, c.AllowRemote, c.AuthToken = "0.0.0.0", true, "secreto"
	})
	f := &reabrirFalso{}
	s.Reabrir = f.Reabrir
	h := s.Routes()

	if rec := post(t, h, "/api/red", `{"activo":false}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("desactivar: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.pedidos) != 1 || f.pedidos[0] != "127.0.0.1" {
		t.Fatalf("Reabrir tenia que recibir 127.0.0.1, recibio %v", f.pedidos)
	}
	cfg, _ := config.Load(paths.Config)
	if cfg.Host != "127.0.0.1" || cfg.AllowRemote || cfg.AuthToken != "secreto" {
		t.Fatalf("config guardada mal: %+v", cfg)
	}
	if rec := get(t, h, "/health", "192.168.1.77:5000", "secreto"); rec.Code != http.StatusForbidden {
		t.Fatalf("con la red apagada, desde la red tiene que ser 403, es %d", rec.Code)
	}
}

func TestRedNoSeCambiaDesdeLaRed(t *testing.T) {
	s, _ := newTestServerAPI(t, func(c *config.Config) {
		c.Host, c.AllowRemote, c.AuthToken = "0.0.0.0", true, "secreto"
	})
	f := &reabrirFalso{}
	s.Reabrir = f.Reabrir
	h := s.Routes()
	if rec := get(t, h, "/api/red", "192.168.1.77:5000", "secreto"); rec.Code != http.StatusForbidden {
		t.Fatalf("GET /api/red desde la red: %d", rec.Code)
	}
	req := `{"activo":false}`
	rec := post(t, h, "/api/red", req, "192.168.1.77:5000")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST /api/red desde la red: %d", rec.Code)
	}
	if len(f.pedidos) != 0 {
		t.Fatalf("no se tenia que reabrir nada: %v", f.pedidos)
	}
}

func TestRedSiReabrirFallaNoGuarda(t *testing.T) {
	s, paths := newTestServerAPI(t, nil)
	f := &reabrirFalso{falla: errors.New("puerto ocupado; sigue en 127.0.0.1")}
	s.Reabrir = f.Reabrir
	h := s.Routes()
	antes, _ := os.ReadFile(paths.Config)

	rec := post(t, h, "/api/red", `{"activo":true}`, "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "puerto ocupado") {
		t.Fatalf("quiero 500 con el motivo, tengo %d %s", rec.Code, rec.Body.String())
	}
	despues, _ := os.ReadFile(paths.Config)
	if string(antes) != string(despues) {
		t.Fatal("si no se pudo abrir el puerto, config.json no puede cambiar")
	}
	if d := leerRed(t, h); d["activo"] != false {
		t.Fatalf("tiene que seguir desactivado: %v", d)
	}
}

func TestRedSiGuardarFallaVuelveAtras(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root se puede escribir en una carpeta de solo lectura")
	}
	s, paths := newTestServerAPI(t, nil)
	f := &reabrirFalso{}
	s.Reabrir = f.Reabrir
	h := s.Routes()
	dir := filepath.Dir(paths.Config)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	rec := post(t, h, "/api/red", `{"activo":true}`, "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("quiero 500, tengo %d %s", rec.Code, rec.Body.String())
	}
	if len(f.pedidos) != 2 || f.pedidos[0] != "0.0.0.0" || f.pedidos[1] != "127.0.0.1" {
		t.Fatalf("tenia que abrir y volver atras, pidio %v", f.pedidos)
	}
	if d := leerRed(t, h); d["activo"] != false {
		t.Fatalf("tiene que seguir desactivado: %v", d)
	}
}

func TestComandoCortafuegos(t *testing.T) {
	if c := comandoCortafuegos("linux", 18743); c != "sudo ufw allow 18743/tcp" {
		t.Fatalf("linux: %q", c)
	}
	if c := comandoCortafuegos("windows", 18743); !strings.Contains(c, "netsh") || !strings.Contains(c, "localport=18743") {
		t.Fatalf("windows: %q", c)
	}
}

func redActiva(t *testing.T) (http.Handler, *Server) {
	t.Helper()
	s, _ := newTestServerAPI(t, func(c *config.Config) {
		c.Host, c.AllowRemote, c.AuthToken = "0.0.0.0", true, "secreto"
	})
	return s.Routes(), s
}

func conexionesDe(t *testing.T, h http.Handler) []any {
	t.Helper()
	c, _ := leerRed(t, h)["conexiones"].([]any)
	return c
}

func TestConectarAnotaLaIPDeOtroAparato(t *testing.T) {
	h, _ := redActiva(t)
	rec := get(t, h, "/conectar", "192.168.1.77:5000", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Conectado") {
		t.Fatalf("quiero 200 con 'Conectado', tengo %d %s", rec.Code, rec.Body.String())
	}
	c := conexionesDe(t, h)
	if len(c) != 1 || c[0].(map[string]any)["ip"] != "192.168.1.77" {
		t.Fatalf("tenia que anotar 192.168.1.77: %v", c)
	}
}

func TestConectarDesdeEstaPCNoCuenta(t *testing.T) {
	h, _ := redActiva(t)
	rec := get(t, h, "/conectar", "127.0.0.1:5000", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "misma PC") {
		t.Fatalf("quiero 200 diciendo 'misma PC', tengo %d %s", rec.Code, rec.Body.String())
	}
	if c := conexionesDe(t, h); len(c) != 0 {
		t.Fatalf("abrirlo desde esta PC no prueba nada de la red: %v", c)
	}
}

func TestConectarConRedApagada(t *testing.T) {
	s, _ := newTestServerAPI(t, nil)
	if rec := get(t, s.Routes(), "/conectar", "127.0.0.1:5000", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("con la red apagada quiero 404, tengo %d", rec.Code)
	}
}

func TestConectarNoEnsenaNadaUtil(t *testing.T) {
	h, _ := redActiva(t)
	cuerpo := get(t, h, "/conectar", "192.168.1.77:5000", "").Body.String()
	for _, prohibido := range []string{"secreto", "auth_token", "printer"} {
		if strings.Contains(cuerpo, prohibido) {
			t.Fatalf("/conectar no puede contener %q:\n%s", prohibido, cuerpo)
		}
	}
}

func TestConexionesSeQuedanEnDiez(t *testing.T) {
	h, _ := redActiva(t)
	for i := 1; i <= 25; i++ {
		get(t, h, "/conectar", "192.168.1."+strconv.Itoa(i)+":5000", "")
	}
	c := conexionesDe(t, h)
	if len(c) != 10 || c[0].(map[string]any)["ip"] != "192.168.1.25" {
		t.Fatalf("quiero las 10 ultimas, la primera .25: %v", c)
	}
}

func TestQRDeLaRedEsSoloLocal(t *testing.T) {
	h, _ := redActiva(t)
	if rec := get(t, h, "/api/red/qr.png", "192.168.1.77:5000", "secreto"); rec.Code != http.StatusForbidden {
		t.Fatalf("el QR desde la red tiene que ser 403, es %d", rec.Code)
	}
}
