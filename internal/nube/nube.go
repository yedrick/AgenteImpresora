// Package nube conecta el agente con un backend en internet.
//
// Un backend en la nube (Laravel, por ejemplo) no puede llamar al agente: el
// agente esta en la red del local, detras de su router. Asi que llama el
// agente. Cada pocos segundos pregunta si hay trabajos, los imprime y
// contesta como salieron; y cada medio minuto avisa de que sigue encendido y
// de como estan las impresoras.
//
// Protocolo, siempre con "Authorization: Bearer <token>":
//
//	GET  {url}/jobs              -> {"jobs":[{"id":"...","type":"ticket","data":{...}}]}
//	POST {url}/jobs/{id}/status  <- {"status":"received|printed|failed","error":"...","local_jobs":[...]}
//	POST {url}/heartbeat         <- {"agent":{...},"stations":[...]}
//
// Un trabajo no se imprime nunca dos veces: se apunta en disco antes de
// mandarlo a la impresora, y lo que ya esta apuntado no se vuelve a imprimir
// aunque el backend lo siga ofreciendo (porque no le llego la respuesta, o
// porque el agente se reinicio).
package nube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/queue"
)

const (
	pollPorDefecto      = 3 * time.Second
	latidoPorDefecto    = 30 * time.Second
	esperaMaxima        = time.Minute
	timeoutPeticion     = 20 * time.Second
	maxRespuesta        = 16 << 20
	retencionVistos     = 7 * 24 * time.Hour
	maxVistos           = 5000
	interrumpidoTras    = time.Minute
	archivoVistos       = "nube.json"
	estadoRecibido      = "received"
	estadoImpreso       = "printed"
	estadoFallido       = "failed"
	direccionInterna    = "127.0.0.1:0"
	cabeceraContenido   = "application/json"
	agenteUsuarioPrefix = "CollaTechAgent/"
)

// rutas son los tipos de trabajo que acepta el backend, con el endpoint
// local que los imprime. Pasar por la API local, en vez de por la cola a
// pelo, hace que valgan las mismas validaciones, estaciones y ajustes que al
// imprimir desde la propia PC.
var rutas = map[string]string{
	"ticket":   "/api/print/ticket",
	"text":     "/api/print/text",
	"template": "/api/print/template",
	"html":     "/api/print/html",
	"layout":   "/api/print/layout",
	"image":    "/api/print/image",
	"logo":     "/api/print/logo",
	"raw":      "/api/print/raw",
}

// Trabajos es lo que el conector necesita de la cola: saber como va cada
// trabajo que mando.
type Trabajos interface {
	Get(id string) (queue.Job, bool)
}

type Opciones struct {
	// Handler es la API local del agente.
	Handler  http.Handler
	Trabajos Trabajos
	// Estaciones devuelve el estado de las impresoras para el latido.
	Estaciones func() any
	Logger     *logs.Logger
	Version    string
	StorageDir string
	// Client permite inyectar uno en las pruebas.
	Client *http.Client
}

// Estado es lo que el panel ensena de la conexion.
type Estado struct {
	Enabled     bool      `json:"enabled"`
	URL         string    `json:"url,omitempty"`
	Connected   bool      `json:"connected"`
	LastContact time.Time `json:"last_contact,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	Received    int       `json:"jobs_received"`
}

// visto es un trabajo del backend que el agente ya recibio.
type visto struct {
	Status   string    `json:"status"`
	Locales  []string  `json:"local_jobs,omitempty"`
	Error    string    `json:"error,omitempty"`
	Reported bool      `json:"reported"`
	Final    bool      `json:"final"`
	At       time.Time `json:"at"`
}

type trabajoNube struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type Conector struct {
	opts   Opciones
	client *http.Client
	inicio time.Time

	// ciclo protege el arranque y la parada del bucle.
	ciclo  sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.Mutex
	cfg    config.Cloud
	estado Estado
	vistos map[string]*visto
	// sucio dice si vistos cambio desde la ultima vez que se guardo. Sin
	// esto se reescribia el archivo en cada vuelta, cada 3 segundos, aunque
	// no hubiera llegado nada: unas 28.000 escrituras al dia para nada.
	sucio bool
}

func New(opts Opciones) *Conector {
	client := opts.Client
	if client == nil {
		client = &http.Client{Timeout: timeoutPeticion}
	}
	c := &Conector{opts: opts, client: client, inicio: time.Now().UTC(), vistos: map[string]*visto{}}
	c.cargar()
	return c
}

// Configurar aplica una configuracion nueva: para el bucle que hubiera y, si
// hay URL y token, arranca otro. Se puede llamar en caliente desde el panel.
func (c *Conector) Configurar(cfg config.Cloud) {
	c.ciclo.Lock()
	defer c.ciclo.Unlock()
	c.pararLocked()

	cfg.URL = strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	cfg.Token = strings.TrimSpace(cfg.Token)
	c.mu.Lock()
	c.cfg = cfg
	c.estado = Estado{Enabled: cfg.Enabled(), URL: cfg.URL, Received: c.estado.Received}
	c.mu.Unlock()
	if !cfg.Enabled() {
		return
	}
	if u, err := url.Parse(cfg.URL); err == nil && u.Scheme == "http" && !esLocal(u.Hostname()) {
		c.log("nube_sin_cifrar", map[string]any{"url": cfg.URL,
			"aviso": "el token viaja sin cifrar: usa https"})
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(2)
	go c.bucleTrabajos(ctx, cfg)
	go c.bucleLatido(ctx, cfg)
	c.log("nube_activada", map[string]any{"url": cfg.URL})
}

// Detener para el conector y espera a que termine lo que estuviera haciendo.
func (c *Conector) Detener() {
	c.ciclo.Lock()
	defer c.ciclo.Unlock()
	c.pararLocked()
}

func (c *Conector) pararLocked() {
	if c.cancel == nil {
		return
	}
	c.cancel()
	c.wg.Wait()
	c.cancel = nil
}

func (c *Conector) Estado() Estado {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.estado
}

// --- Bucles ----------------------------------------------------------------

func (c *Conector) bucleTrabajos(ctx context.Context, cfg config.Cloud) {
	defer c.wg.Done()
	cada := segundos(cfg.PollSeconds, pollPorDefecto)
	fallos := 0
	for {
		espera := cada
		if err := c.ronda(ctx, cfg); err != nil {
			fallos++
			espera = retroceso(cada, fallos)
		} else {
			fallos = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(espera):
		}
	}
}

func (c *Conector) bucleLatido(ctx context.Context, cfg config.Cloud) {
	defer c.wg.Done()
	cada := segundos(cfg.HeartbeatSeconds, latidoPorDefecto)
	for {
		c.latido(ctx, cfg)
		select {
		case <-ctx.Done():
			return
		case <-time.After(cada):
		}
	}
}

// retroceso alarga la espera tras cada fallo seguido, para no martillear un
// backend caido, sin pasar de un minuto: cuando vuelva, el local tiene que
// enterarse pronto.
func retroceso(base time.Duration, fallos int) time.Duration {
	d := base
	for i := 1; i < fallos && d < esperaMaxima; i++ {
		d *= 2
	}
	if d > esperaMaxima {
		d = esperaMaxima
	}
	return d
}

func segundos(n int, def time.Duration) time.Duration {
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}

// ronda es una vuelta del bucle: informa de lo pendiente y recoge trabajos
// nuevos.
func (c *Conector) ronda(ctx context.Context, cfg config.Cloud) error {
	c.seguir()
	c.informarPendientes(ctx, cfg)

	codigo, cuerpo, err := c.peticion(ctx, cfg, http.MethodGet, "/jobs", nil)
	if err != nil {
		return err
	}
	if codigo != http.StatusOK {
		return c.fallo(fmt.Errorf("GET /jobs respondio %d: %s", codigo, recortar(cuerpo)))
	}
	var resp struct {
		Jobs []trabajoNube `json:"jobs"`
	}
	if err := json.Unmarshal(cuerpo, &resp); err != nil {
		return c.fallo(fmt.Errorf("GET /jobs no devolvio el formato esperado ({\"jobs\":[...]}): %v", err))
	}
	for _, t := range resp.Jobs {
		if ctx.Err() != nil {
			return nil
		}
		c.procesar(ctx, cfg, t)
	}
	c.guardar()
	return nil
}

// procesar imprime un trabajo del backend, salvo que ya se hubiera recibido.
func (c *Conector) procesar(ctx context.Context, cfg config.Cloud, t trabajoNube) {
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		c.log("nube_trabajo_sin_id", map[string]any{"type": t.Type})
		return
	}
	c.mu.Lock()
	ya, repetido := c.vistos[t.ID]
	if repetido {
		// El backend lo sigue ofreciendo: no le llego la respuesta. Se le
		// repite, pero no se vuelve a imprimir.
		ya.Reported = false
	}
	c.mu.Unlock()
	if repetido {
		c.informar(ctx, cfg, t.ID)
		return
	}

	ruta, ok := rutas[strings.ToLower(strings.TrimSpace(t.Type))]
	if !ok {
		c.registrar(t.ID, &visto{Status: estadoFallido, Final: true, At: time.Now().UTC(),
			Error: fmt.Sprintf("tipo de trabajo %q desconocido; los validos son: %s", t.Type, tiposValidos())})
		c.informar(ctx, cfg, t.ID)
		return
	}

	// Se apunta ANTES de imprimir y se guarda en disco: si el agente se
	// cae justo despues de mandar el ticket a la impresora, al volver no lo
	// imprime otra vez.
	c.registrar(t.ID, &visto{Status: estadoRecibido, At: time.Now().UTC()})
	c.guardar()

	ids, transitorio, err := c.despachar(ruta, t.Data)
	switch {
	case transitorio:
		// Cola llena o limite de ritmo: no es culpa del trabajo. Se olvida
		// para que el backend lo vuelva a ofrecer en la proxima vuelta.
		c.mu.Lock()
		delete(c.vistos, t.ID)
		c.sucio = true
		c.mu.Unlock()
		c.log("nube_trabajo_aplazado", map[string]any{"id": t.ID, "motivo": err.Error()})
		return
	case err != nil:
		c.actualizar(t.ID, func(v *visto) {
			v.Status, v.Error, v.Final, v.Reported = estadoFallido, err.Error(), true, false
		})
	default:
		c.actualizar(t.ID, func(v *visto) { v.Locales = ids })
	}
	c.mu.Lock()
	c.estado.Received++
	c.mu.Unlock()
	c.log("nube_trabajo_recibido", map[string]any{"id": t.ID, "type": t.Type, "local_jobs": ids})
	c.informar(ctx, cfg, t.ID)
}

// despachar manda el trabajo a la API local como si llegara de la propia PC.
func (c *Conector) despachar(ruta string, datos json.RawMessage) (ids []string, transitorio bool, err error) {
	if len(bytes.TrimSpace(datos)) == 0 {
		return nil, false, errors.New(`el trabajo no trae "data"`)
	}
	req, err := http.NewRequest(http.MethodPost, ruta, bytes.NewReader(datos))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", cabeceraContenido)
	req.RemoteAddr = direccionInterna
	rec := &grabador{cabeceras: http.Header{}, codigo: http.StatusOK}
	c.opts.Handler.ServeHTTP(rec, req)

	var resp struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(rec.cuerpo.Bytes(), &resp)
	switch rec.codigo {
	case http.StatusAccepted:
	case http.StatusServiceUnavailable, http.StatusTooManyRequests:
		return nil, true, fmt.Errorf("el agente esta ocupado (%d): %s", rec.codigo, resp.Error)
	default:
		msg := resp.Error
		if msg == "" {
			msg = recortar(rec.cuerpo.Bytes())
		}
		return nil, false, fmt.Errorf("%s", msg)
	}

	// Un destino devuelve un trabajo; una estacion con varias impresoras,
	// una lista.
	var uno struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(resp.Data, &uno) == nil && uno.ID != "" {
		return []string{uno.ID}, false, nil
	}
	var varios []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp.Data, &varios); err != nil || len(varios) == 0 {
		return nil, false, errors.New("el agente encolo el trabajo pero no devolvio su id")
	}
	for _, v := range varios {
		ids = append(ids, v.ID)
	}
	return ids, false, nil
}

// seguir mira en la cola como van los trabajos recibidos y cierra los que
// terminaron.
func (c *Conector) seguir() {
	c.mu.Lock()
	defer c.mu.Unlock()
	ahora := time.Now().UTC()
	for _, v := range c.vistos {
		if v.Final {
			continue
		}
		if len(v.Locales) == 0 {
			// Apuntado pero nunca llego a la cola: el agente se paro entre
			// una cosa y la otra. No se reimprime (podria haber salido), se
			// avisa.
			if ahora.Sub(v.At) > interrumpidoTras {
				c.sucio = true
				v.Status, v.Final, v.Reported = estadoFallido, true, false
				v.Error = "el agente se reinicio mientras lo recibia; no se sabe si salio, revisalo y vuelve a enviarlo si hace falta"
			}
			continue
		}
		terminados, fallos := 0, []string{}
		for _, id := range v.Locales {
			job, ok := c.opts.Trabajos.Get(id)
			switch {
			case !ok:
				terminados++
				fallos = append(fallos, id+": ya no esta en el historial del agente")
			case job.Status == queue.Completed:
				terminados++
			case job.Status == queue.Failed:
				terminados++
				fallos = append(fallos, job.Printer+": "+job.Error)
			}
		}
		if terminados < len(v.Locales) {
			continue
		}
		c.sucio = true
		v.Final, v.Reported = true, false
		if len(fallos) == 0 {
			v.Status, v.Error = estadoImpreso, ""
		} else {
			v.Status, v.Error = estadoFallido, strings.Join(fallos, "; ")
		}
	}
}

func (c *Conector) informarPendientes(ctx context.Context, cfg config.Cloud) {
	c.mu.Lock()
	var ids []string
	for id, v := range c.vistos {
		if !v.Reported {
			ids = append(ids, id)
		}
	}
	c.mu.Unlock()
	sort.Strings(ids)
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		c.informar(ctx, cfg, id)
	}
}

// informar le cuenta al backend como va un trabajo.
func (c *Conector) informar(ctx context.Context, cfg config.Cloud, id string) {
	c.mu.Lock()
	v, ok := c.vistos[id]
	if !ok || v.Reported {
		c.mu.Unlock()
		return
	}
	cuerpo := map[string]any{"status": v.Status, "error": v.Error, "local_jobs": c.localesLocked(v)}
	final := v.Final
	c.mu.Unlock()

	codigo, resp, err := c.peticion(ctx, cfg, http.MethodPost, "/jobs/"+url.PathEscape(id)+"/status", cuerpo)
	if err != nil {
		return
	}
	switch {
	case codigo >= 200 && codigo < 300:
	case codigo == http.StatusNotFound || codigo == http.StatusGone:
		// El backend ya no lo conoce: no tiene sentido seguir insistiendo.
		c.log("nube_trabajo_desconocido", map[string]any{"id": id, "status": codigo})
	default:
		c.log("nube_informe_rechazado", map[string]any{"id": id, "status": codigo, "respuesta": recortar(resp)})
		return
	}
	c.actualizar(id, func(v *visto) {
		// Solo se da por informado el estado que se mando: si entretanto
		// paso a final, queda pendiente de contarlo.
		if v.Final == final {
			v.Reported = true
		}
	})
	c.guardar()
}

func (c *Conector) localesLocked(v *visto) []map[string]any {
	out := make([]map[string]any, 0, len(v.Locales))
	for _, id := range v.Locales {
		item := map[string]any{"id": id}
		if job, ok := c.opts.Trabajos.Get(id); ok {
			item["printer"] = job.Printer
			item["status"] = job.Status
			item["attempts"] = job.Attempts
			if job.Error != "" {
				item["error"] = job.Error
			}
		}
		out = append(out, item)
	}
	return out
}

// latido avisa de que el agente sigue encendido y de como estan las
// impresoras. Es lo que permite al backend saber, sin imprimir, si puede
// contar con este local.
func (c *Conector) latido(ctx context.Context, cfg config.Cloud) {
	host, _ := os.Hostname()
	var estaciones any = []any{}
	if c.opts.Estaciones != nil {
		estaciones = c.opts.Estaciones()
	}
	cuerpo := map[string]any{
		"agent": map[string]any{
			"version":        c.opts.Version,
			"hostname":       host,
			"os":             runtime.GOOS,
			"started_at":     c.inicio,
			"uptime_seconds": int(time.Since(c.inicio).Seconds()),
		},
		"stations": estaciones,
	}
	codigo, resp, err := c.peticion(ctx, cfg, http.MethodPost, "/heartbeat", cuerpo)
	if err != nil {
		return
	}
	if codigo < 200 || codigo >= 300 {
		c.fallo(fmt.Errorf("POST /heartbeat respondio %d: %s", codigo, recortar(resp)))
	}
}

// --- HTTP ------------------------------------------------------------------

// peticion habla con el backend y lleva la cuenta de si hay conexion.
func (c *Conector) peticion(ctx context.Context, cfg config.Cloud, metodo, ruta string, cuerpo any) (int, []byte, error) {
	var body io.Reader
	if cuerpo != nil {
		b, err := json.Marshal(cuerpo)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, cfg.URL+ruta, body)
	if err != nil {
		return 0, nil, c.fallo(err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", cabeceraContenido)
	req.Header.Set("User-Agent", agenteUsuarioPrefix+c.opts.Version)
	if cuerpo != nil {
		req.Header.Set("Content-Type", cabeceraContenido)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, c.fallo(fmt.Errorf("no se pudo conectar con %s: %v", cfg.URL, err))
	}
	defer resp.Body.Close()
	leido, err := io.ReadAll(io.LimitReader(resp.Body, maxRespuesta))
	if err != nil {
		return 0, nil, c.fallo(err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return resp.StatusCode, leido, c.fallo(fmt.Errorf("el backend rechazo el token (%d): revisa el token de este local", resp.StatusCode))
	}
	c.conectado()
	return resp.StatusCode, leido, nil
}

func (c *Conector) conectado() {
	c.mu.Lock()
	antes := c.estado.Connected
	c.estado.Connected, c.estado.LastError, c.estado.LastContact = true, "", time.Now().UTC()
	c.mu.Unlock()
	if !antes {
		c.log("nube_conectada", nil)
	}
}

// fallo apunta el error y lo registra solo cuando cambia: con el backend
// caido, el bucle reintenta sin parar y llenaria el log con la misma linea.
func (c *Conector) fallo(err error) error {
	c.mu.Lock()
	cambio := c.estado.LastError != err.Error()
	c.estado.Connected, c.estado.LastError = false, err.Error()
	c.mu.Unlock()
	if cambio && c.opts.Logger != nil {
		c.opts.Logger.Error("nube_sin_conexion", map[string]any{"error": err.Error()})
	}
	return err
}

// --- Registro de trabajos vistos -------------------------------------------

func (c *Conector) registrar(id string, v *visto) {
	c.mu.Lock()
	c.vistos[id] = v
	c.sucio = true
	c.mu.Unlock()
}

func (c *Conector) actualizar(id string, f func(*visto)) {
	c.mu.Lock()
	if v, ok := c.vistos[id]; ok {
		f(v)
		c.sucio = true
	}
	c.mu.Unlock()
}

func (c *Conector) ruta() string {
	dir := c.opts.StorageDir
	if dir == "" {
		dir = "storage"
	}
	return filepath.Join(dir, archivoVistos)
}

func (c *Conector) cargar() {
	b, err := os.ReadFile(c.ruta())
	if err != nil {
		return
	}
	var vistos map[string]*visto
	if err := json.Unmarshal(b, &vistos); err != nil {
		c.log("nube_registro_ilegible", map[string]any{"error": err.Error()})
		return
	}
	for id, v := range vistos {
		if v != nil {
			c.vistos[id] = v
		}
	}
}

// guardar escribe el registro en disco si cambio. Antes se purgan los
// trabajos cerrados e informados que ya son viejos.
func (c *Conector) guardar() {
	c.mu.Lock()
	limite := time.Now().UTC().Add(-retencionVistos)
	for id, v := range c.vistos {
		if v.Final && v.Reported && v.At.Before(limite) {
			delete(c.vistos, id)
			c.sucio = true
		}
	}
	if !c.sucio {
		c.mu.Unlock()
		return
	}
	if len(c.vistos) > maxVistos {
		ids := make([]string, 0, len(c.vistos))
		for id := range c.vistos {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return c.vistos[ids[i]].At.Before(c.vistos[ids[j]].At) })
		for _, id := range ids[:len(ids)-maxVistos] {
			if v := c.vistos[id]; v.Final && v.Reported {
				delete(c.vistos, id)
			}
		}
	}
	b, err := json.Marshal(c.vistos)
	c.sucio = false
	c.mu.Unlock()
	if err != nil {
		return
	}
	if err := escribir(c.ruta(), b); err != nil {
		// Queda pendiente para la proxima vuelta.
		c.mu.Lock()
		c.sucio = true
		c.mu.Unlock()
		c.log("nube_registro_no_guardado", map[string]any{"error": err.Error()})
	}
}

// escribir reemplaza el archivo de una vez: un corte de luz a mitad deja el
// anterior entero, no uno a medias.
func escribir(ruta string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return err
	}
	tmp := ruta + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, ruta)
}

// --- Utilidades ------------------------------------------------------------

func (c *Conector) log(evento string, datos map[string]any) {
	if c.opts.Logger != nil {
		c.opts.Logger.Info(evento, datos)
	}
}

func tiposValidos() string {
	tipos := make([]string, 0, len(rutas))
	for t := range rutas {
		tipos = append(tipos, t)
	}
	sort.Strings(tipos)
	return strings.Join(tipos, ", ")
}

func recortar(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

func esLocal(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// grabador recoge la respuesta de la API local.
type grabador struct {
	cabeceras http.Header
	codigo    int
	cuerpo    bytes.Buffer
	escrito   bool
}

func (g *grabador) Header() http.Header { return g.cabeceras }

func (g *grabador) WriteHeader(codigo int) {
	if !g.escrito {
		g.codigo, g.escrito = codigo, true
	}
}

func (g *grabador) Write(b []byte) (int, error) {
	g.escrito = true
	return g.cuerpo.Write(b)
}
