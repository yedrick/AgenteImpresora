package nube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"collatech-agent/internal/config"
	"collatech-agent/internal/queue"
)

// backend simula el Laravel: ofrece los trabajos que tenga pendientes hasta
// que el agente le informa de que terminaron, y apunta todo lo que recibe.
type backend struct {
	t     *testing.T
	token string

	mu        sync.Mutex
	pendiente []trabajoNube
	informes  map[string][]string
	latidos   []map[string]any
}

func nuevoBackend(t *testing.T, trabajos ...trabajoNube) (*backend, *httptest.Server) {
	b := &backend{t: t, token: "secreto", pendiente: trabajos, informes: map[string][]string{}}
	srv := httptest.NewServer(http.HandlerFunc(b.servir))
	t.Cleanup(srv.Close)
	return b, srv
}

func (b *backend) servir(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+b.token {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/jobs":
		_ = json.NewEncoder(w).Encode(map[string]any{"jobs": b.pendiente})
	case r.Method == http.MethodPost && r.URL.Path == "/heartbeat":
		var cuerpo map[string]any
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		b.latidos = append(b.latidos, cuerpo)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/jobs/") && strings.HasSuffix(r.URL.Path, "/status"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/jobs/"), "/status")
		var cuerpo struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		_ = json.NewDecoder(r.Body).Decode(&cuerpo)
		b.informes[id] = append(b.informes[id], cuerpo.Status+"|"+cuerpo.Error)
		// Como Laravel: un trabajo informado deja de estar pendiente.
		if cuerpo.Status == estadoImpreso || cuerpo.Status == estadoFallido {
			var quedan []trabajoNube
			for _, t := range b.pendiente {
				if t.ID != id {
					quedan = append(quedan, t)
				}
			}
			b.pendiente = quedan
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (b *backend) estados(id string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.informes[id]...)
}

// agenteFalso hace de API local y de cola: cuenta las impresiones y deja
// elegir como acaban.
type agenteFalso struct {
	mu       sync.Mutex
	llamadas int
	codigo   int
	estado   queue.Status
	errorTxt string
}

func (a *agenteFalso) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.RemoteAddr != direccionInterna {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	cuerpo, _ := io.ReadAll(r.Body)
	if !json.Valid(cuerpo) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	codigo := a.codigo
	if codigo == 0 {
		codigo = http.StatusAccepted
	}
	w.WriteHeader(codigo)
	if codigo != http.StatusAccepted {
		_, _ = w.Write([]byte(`{"ok":false,"error":"no se puede ahora"}`))
		return
	}
	a.llamadas++
	_, _ = w.Write([]byte(`{"ok":true,"data":{"id":"local-1","printer":"caja","status":"Pending"}}`))
}

func (a *agenteFalso) Get(id string) (queue.Job, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id != "local-1" {
		return queue.Job{}, false
	}
	estado := a.estado
	if estado == "" {
		estado = queue.Completed
	}
	return queue.Job{ID: id, Printer: "caja", Status: estado, Error: a.errorTxt}, true
}

func (a *agenteFalso) impresiones() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.llamadas
}

func conector(t *testing.T, srv *httptest.Server, dir string, a *agenteFalso) (*Conector, config.Cloud) {
	t.Helper()
	c := New(Opciones{
		Handler: a, Trabajos: a, StorageDir: dir, Version: "test",
		Estaciones: func() any { return []map[string]any{{"name": "caja", "ready": true}} },
	})
	return c, config.Cloud{URL: srv.URL, Token: "secreto"}
}

var ticket = trabajoNube{ID: "t-1", Type: "ticket", Data: json.RawMessage(`{"printer":"caja","lines":[{"text":"hola"}]}`)}

// El backend sigue ofreciendo el trabajo hasta que le llega el informe
// final. Mientras tanto el agente no puede volver a imprimirlo.
func TestImprimeUnaSolaVez(t *testing.T) {
	b, srv := nuevoBackend(t, ticket)
	a := &agenteFalso{estado: queue.Printing}
	c, cfg := conector(t, srv, t.TempDir(), a)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := c.ronda(ctx, cfg); err != nil {
			t.Fatal(err)
		}
	}
	if n := a.impresiones(); n != 1 {
		t.Fatalf("se imprimio %d veces", n)
	}
	if got := b.estados("t-1"); len(got) == 0 || got[0] != "received|" {
		t.Fatalf("el backend tenia que saber que se recibio: %v", got)
	}

	a.mu.Lock()
	a.estado = queue.Completed
	a.mu.Unlock()
	if err := c.ronda(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	got := b.estados("t-1")
	if got[len(got)-1] != "printed|" {
		t.Fatalf("al terminar tenia que informar printed: %v", got)
	}
	if n := a.impresiones(); n != 1 {
		t.Fatalf("se imprimio %d veces", n)
	}
}

// Tras reiniciar, lo ya recibido sigue apuntado: el agente nuevo no lo
// reimprime aunque el backend se lo vuelva a ofrecer.
func TestNoReimprimeTrasReiniciar(t *testing.T) {
	_, srv := nuevoBackend(t, ticket)
	dir := t.TempDir()
	a := &agenteFalso{estado: queue.Printing}

	c, cfg := conector(t, srv, dir, a)
	if err := c.ronda(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, archivoVistos)); err != nil {
		t.Fatalf("no se guardo el registro: %v", err)
	}

	otro, cfg := conector(t, srv, dir, a)
	if err := otro.ronda(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if n := a.impresiones(); n != 1 {
		t.Fatalf("tras reiniciar se imprimio otra vez: %d impresiones", n)
	}
}

// Si el agente se paro entre apuntarlo y mandarlo a la cola, no se sabe si
// salio: se avisa al backend en vez de reimprimir.
func TestTrabajoInterrumpidoSeAvisa(t *testing.T) {
	b, srv := nuevoBackend(t, ticket)
	dir := t.TempDir()
	viejo := map[string]*visto{"t-1": {Status: estadoRecibido, At: time.Now().Add(-2 * interrumpidoTras)}}
	raw, _ := json.Marshal(viejo)
	if err := os.WriteFile(filepath.Join(dir, archivoVistos), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	a := &agenteFalso{}
	c, cfg := conector(t, srv, dir, a)
	if err := c.ronda(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if a.impresiones() != 0 {
		t.Fatal("reimprimio un trabajo que pudo haber salido")
	}
	got := b.estados("t-1")
	if len(got) == 0 || !strings.HasPrefix(got[len(got)-1], "failed|el agente se reinicio") {
		t.Fatalf("tenia que avisar del reinicio: %v", got)
	}
}

func TestTipoDesconocidoFalla(t *testing.T) {
	b, srv := nuevoBackend(t, trabajoNube{ID: "t-2", Type: "fax", Data: json.RawMessage(`{}`)})
	a := &agenteFalso{}
	c, cfg := conector(t, srv, t.TempDir(), a)
	if err := c.ronda(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	got := b.estados("t-2")
	if len(got) != 1 || !strings.Contains(got[0], "failed|tipo de trabajo \"fax\" desconocido") {
		t.Fatalf("informe: %v", got)
	}
	if a.impresiones() != 0 {
		t.Fatal("no tenia que imprimir nada")
	}
}

// Con la cola llena el trabajo no falla: se deja para la proxima vuelta.
func TestColaLlenaAplaza(t *testing.T) {
	b, srv := nuevoBackend(t, ticket)
	a := &agenteFalso{codigo: http.StatusServiceUnavailable}
	c, cfg := conector(t, srv, t.TempDir(), a)
	ctx := context.Background()
	if err := c.ronda(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if got := b.estados("t-1"); len(got) != 0 {
		t.Fatalf("no tenia que informar nada todavia: %v", got)
	}

	a.mu.Lock()
	a.codigo = 0
	a.mu.Unlock()
	if err := c.ronda(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := c.ronda(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if got := b.estados("t-1"); got[len(got)-1] != "printed|" {
		t.Fatalf("en la segunda vuelta tenia que imprimirse: %v", got)
	}
}

// El fallo de la impresora llega al backend con el motivo.
func TestFalloDeImpresoraSeInforma(t *testing.T) {
	b, srv := nuevoBackend(t, ticket)
	a := &agenteFalso{estado: queue.Failed, errorTxt: "sin papel"}
	c, cfg := conector(t, srv, t.TempDir(), a)
	ctx := context.Background()
	_ = c.ronda(ctx, cfg)
	_ = c.ronda(ctx, cfg)
	got := b.estados("t-1")
	if got[len(got)-1] != "failed|caja: sin papel" {
		t.Fatalf("informe: %v", got)
	}
}

func TestTokenRechazado(t *testing.T) {
	_, srv := nuevoBackend(t)
	c, cfg := conector(t, srv, t.TempDir(), &agenteFalso{})
	cfg.Token = "otro"
	if err := c.ronda(context.Background(), cfg); err == nil {
		t.Fatal("un token rechazado tiene que ser un error")
	}
	if e := c.Estado(); e.Connected || !strings.Contains(e.LastError, "rechazo el token") {
		t.Fatalf("estado: %+v", e)
	}
}

// El latido lleva la version y el estado de las estaciones: es lo que deja
// al backend saber, sin imprimir, si puede contar con el local.
func TestLatido(t *testing.T) {
	b, srv := nuevoBackend(t)
	c, cfg := conector(t, srv, t.TempDir(), &agenteFalso{})
	c.latido(context.Background(), cfg)

	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.latidos) != 1 {
		t.Fatalf("latidos: %d", len(b.latidos))
	}
	agente, _ := b.latidos[0]["agent"].(map[string]any)
	if agente["version"] != "test" {
		t.Fatalf("el latido no trae la version: %v", b.latidos[0])
	}
	if est, _ := b.latidos[0]["stations"].([]any); len(est) != 1 {
		t.Fatalf("el latido no trae las estaciones: %v", b.latidos[0])
	}
	if e := c.Estado(); !e.Connected || e.LastContact.IsZero() {
		t.Fatalf("tras un latido correcto tiene que constar como conectado: %+v", e)
	}
}

// Configurar arranca y para los bucles en caliente, sin colgarse.
func TestConfigurarEnCaliente(t *testing.T) {
	b, srv := nuevoBackend(t)
	c, cfg := conector(t, srv, t.TempDir(), &agenteFalso{})
	cfg.PollSeconds, cfg.HeartbeatSeconds = 1, 1
	c.Configurar(cfg)

	deadline := time.Now().Add(5 * time.Second)
	for {
		b.mu.Lock()
		n := len(b.latidos)
		b.mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no llego ningun latido")
		}
		time.Sleep(20 * time.Millisecond)
	}

	listo := make(chan struct{})
	go func() {
		c.Configurar(config.Cloud{})
		close(listo)
	}()
	select {
	case <-listo:
	case <-time.After(5 * time.Second):
		t.Fatal("apagar el conector se quedo colgado")
	}
	if c.Estado().Enabled {
		t.Fatal("sin URL tiene que quedar apagado")
	}
}

func TestRetroceso(t *testing.T) {
	base := 3 * time.Second
	if d := retroceso(base, 1); d != base {
		t.Fatalf("primer fallo: %v", d)
	}
	if d := retroceso(base, 3); d != 12*time.Second {
		t.Fatalf("tercer fallo: %v", d)
	}
	if d := retroceso(base, 50); d != esperaMaxima {
		t.Fatalf("no tiene que pasar del maximo: %v", d)
	}
}

// Sin trabajos no se escribe nada en disco: el agente pregunta cada 3
// segundos todo el dia y antes reescribia el registro en cada vuelta.
func TestSinCambiosNoEscribeEnDisco(t *testing.T) {
	_, srv := nuevoBackend(t, ticket)
	dir := t.TempDir()
	a := &agenteFalso{}
	c, cfg := conector(t, srv, dir, a)
	ctx := context.Background()
	_ = c.ronda(ctx, cfg) // recibe e informa
	_ = c.ronda(ctx, cfg) // termina e informa printed

	ruta := filepath.Join(dir, archivoVistos)
	antes, err := os.Stat(ruta)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	for i := 0; i < 5; i++ {
		_ = c.ronda(ctx, cfg)
	}
	despues, err := os.Stat(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if !despues.ModTime().Equal(antes.ModTime()) {
		t.Fatal("se reescribio el registro sin que hubiera cambios")
	}
}
