package actualizar

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	nombreManifiesto = "manifiesto.json"
	nombreFirma      = "manifiesto.json.sig"
	nombreOrden      = "orden.json"
	nombreResultado  = "resultado.json"

	maxManifiesto = 1 << 20
	maxEjecutable = 200 << 20
)

// Orden es lo que el servicio deja escrito para que el aplicador, fuera del
// servicio, haga el cambio. Todas las rutas son absolutas.
type Orden struct {
	Version    string `json:"version"`
	Plataforma string `json:"plataforma"`
	Nuevo      string `json:"nuevo"`
	Manifiesto string `json:"manifiesto"`
	Firma      string `json:"firma"`
	Config     string `json:"config"`
	Datos      string `json:"datos"`
	Servicio   string `json:"servicio"`
}

// Estado es la fase de la actualizacion en curso, para el panel.
type Estado struct {
	// Fase: "", "descargando", "verificando", "esperando_cola", "instalando"
	// o "error".
	Fase    string `json:"fase"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Resultado es como acabo la ultima actualizacion. Lo escribe el aplicador
// y lo lee la version que arranque despues.
type Resultado struct {
	Desde string    `json:"desde"`
	Hasta string    `json:"hasta"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
	Fecha time.Time `json:"fecha"`
}

// Nueva es una version mas nueva que la instalada.
type Nueva struct {
	Version  string `json:"version"`
	URLNotas string `json:"url_notas"`
	// Manual es el texto del manifiesto, si esta version se instala a mano.
	Manual string `json:"manual,omitempty"`
	// ManualMotivo, si no esta vacio, dice por que no se puede instalar con
	// el boton.
	ManualMotivo string `json:"manual_motivo,omitempty"`
}

// Disponible es el resultado de buscar.
type Disponible struct {
	VersionActual string `json:"version_actual"`
	Nueva         *Nueva `json:"nueva"`
}

// Opciones configura un Actualizador.
type Opciones struct {
	Version    string
	DataDir    string
	ConfigPath string
	Servicio   string
	Fuente     Fuente
	Sistema    Sistema
	Claves     []ed25519.PublicKey
	// ColaVacia dice si no queda nada pendiente de imprimir.
	ColaVacia func() bool
}

// Actualizador busca e instala versiones nuevas, una a la vez.
type Actualizador struct {
	o          Opciones
	esperaCola time.Duration
	// plazoInstalando: si en este tiempo nadie recoge la orden, se da por
	// fallida.
	plazoInstalando time.Duration

	mu         sync.Mutex
	estado     Estado
	encontrada *Publicacion
	// desde cuando esta en "instalando"
	instalandoDesde time.Time
}

func NuevoActualizador(o Opciones) *Actualizador {
	return &Actualizador{o: o, esperaCola: 30 * time.Second, plazoInstalando: 3 * time.Minute}
}

func (a *Actualizador) carpeta() string {
	return filepath.Join(a.o.DataDir, "actualizacion")
}

// Soportado dice si se puede actualizar con el boton en esta instalacion.
func (a *Actualizador) Soportado() (bool, string) {
	if a.o.Sistema == nil {
		return false, "este sistema no se puede actualizar con el boton"
	}
	return a.o.Sistema.Soportado()
}

// Buscar consulta la ultima publicacion. Si es mas nueva, descarga y
// verifica su manifiesto para saber si se puede instalar con el boton.
func (a *Actualizador) Buscar(ctx context.Context) (Disponible, error) {
	d := Disponible{VersionActual: a.o.Version}
	p, err := a.o.Fuente.Ultima(ctx)
	if err != nil {
		return d, err
	}
	if !EsMayor(p.Version, a.o.Version) {
		return d, nil
	}
	nueva := &Nueva{Version: p.Version, URLNotas: p.URLNotas}
	dir, err := os.MkdirTemp("", "collatech-buscar")
	if err != nil {
		return d, err
	}
	defer os.RemoveAll(dir)
	m, err := a.manifiesto(ctx, p, dir)
	if err != nil {
		return d, err
	}
	switch {
	case m.Manual != "":
		nueva.Manual = m.Manual
		nueva.ManualMotivo = "Esta version se instala a mano: " + m.Manual
	case m.Minima != "" && Comparar(a.o.Version, m.Minima) < 0:
		nueva.ManualMotivo = fmt.Sprintf("Desde la %s hay que instalar a mano hasta la %s; despues ya vale el boton.", a.o.Version, m.Minima)
	default:
		if _, ok := m.Archivos[Plataforma()]; !ok {
			nueva.ManualMotivo = "Esta version no trae ejecutable para " + Plataforma() + ": se instala a mano."
		}
	}
	d.Nueva = nueva

	a.mu.Lock()
	if nueva.ManualMotivo == "" {
		a.encontrada = &p
	} else {
		a.encontrada = nil
	}
	a.mu.Unlock()
	return d, nil
}

// manifiesto descarga el manifiesto y su firma a dir y los verifica.
func (a *Actualizador) manifiesto(ctx context.Context, p Publicacion, dir string) (Manifiesto, error) {
	var m Manifiesto
	urlM, urlF := p.Recursos[nombreManifiesto], p.Recursos[nombreFirma]
	if urlM == "" || urlF == "" {
		return m, fmt.Errorf("la version %s no trae manifiesto firmado: no se puede instalar con el boton", p.Version)
	}
	rutaM, rutaF := filepath.Join(dir, nombreManifiesto), filepath.Join(dir, nombreFirma)
	if err := a.o.Fuente.Descargar(ctx, urlM, rutaM, maxManifiesto); err != nil {
		return m, err
	}
	if err := a.o.Fuente.Descargar(ctx, urlF, rutaF, maxManifiesto); err != nil {
		return m, err
	}
	datos, err := os.ReadFile(rutaM)
	if err != nil {
		return m, err
	}
	firma, err := os.ReadFile(rutaF)
	if err != nil {
		return m, err
	}
	m, err = VerificarManifiesto(datos, firma, a.o.Claves)
	if err != nil {
		return m, err
	}
	if m.Version != p.Version {
		return m, fmt.Errorf("el manifiesto dice %s pero la publicacion es %s: no se instala", m.Version, p.Version)
	}
	return m, nil
}

// Instalar empieza a instalar la version encontrada al buscar. Vuelve en
// cuanto empieza: el progreso se lee con Estado.
func (a *Actualizador) Instalar(version string) error {
	if ok, motivo := a.Soportado(); !ok {
		return errors.New(motivo)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch a.revisarLocked().Fase {
	case "descargando", "verificando", "esperando_cola", "instalando":
		return errors.New("ya hay una actualizacion en curso")
	}
	if a.encontrada == nil || a.encontrada.Version != version {
		return fmt.Errorf("busca la actualizacion antes de instalar la %s", version)
	}
	p := *a.encontrada
	a.estado = Estado{Fase: "descargando", Version: version}
	go a.instalar(p)
	return nil
}

// Estado devuelve la fase de la actualizacion en curso.
//
// Si todo va bien, este proceso no llega a ver el final: el aplicador lo
// para y arranca la version nueva. Si sigue vivo en "instalando" es que el
// aplicador fallo antes de reiniciar (lo dice resultado.json) o que nadie
// recogio la orden; en los dos casos se pasa a error para poder reintentar.
func (a *Actualizador) Estado() Estado {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.revisarLocked()
}

func (a *Actualizador) revisarLocked() Estado {
	if a.estado.Fase != "instalando" {
		return a.estado
	}
	if r := a.UltimoResultado(); r != nil && !r.OK && r.Fecha.After(a.instalandoDesde) {
		a.estado.Fase, a.estado.Error = "error", r.Error
		return a.estado
	}
	if time.Since(a.instalandoDesde) > a.plazoInstalando {
		// Si el aplicador esta trabajando (aparto la orden), se le espera.
		if _, err := os.Stat(filepath.Join(a.carpeta(), "orden.en-curso.json")); err == nil {
			return a.estado
		}
		os.Remove(filepath.Join(a.carpeta(), nombreOrden))
		a.estado.Fase = "error"
		a.estado.Error = "la actualizacion no arranco: nadie recogio la orden. En Linux revisa: systemctl status collatech-agent-actualizar.path"
	}
	return a.estado
}

// UltimoResultado lee como acabo la ultima actualizacion, si hubo alguna.
func (a *Actualizador) UltimoResultado() *Resultado {
	b, err := os.ReadFile(filepath.Join(a.carpeta(), nombreResultado))
	if err != nil {
		return nil
	}
	var r Resultado
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

func (a *Actualizador) fase(f string) {
	a.mu.Lock()
	a.estado.Fase = f
	if f == "instalando" {
		a.instalandoDesde = time.Now()
	}
	a.mu.Unlock()
}

func (a *Actualizador) fallar(err error) {
	a.mu.Lock()
	a.estado.Fase = "error"
	a.estado.Error = err.Error()
	a.mu.Unlock()
}

func (a *Actualizador) instalar(p Publicacion) {
	if err := a.preparar(p); err != nil {
		a.fallar(err)
	}
}

func (a *Actualizador) preparar(p Publicacion) error {
	ctx := context.Background()
	dir := a.carpeta()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	m, err := a.manifiesto(ctx, p, dir)
	if err != nil {
		return err
	}
	archivo, ok := m.Archivos[Plataforma()]
	if !ok {
		return fmt.Errorf("la version %s no trae ejecutable para %s", p.Version, Plataforma())
	}
	url := p.Recursos[archivo.Nombre]
	if url == "" {
		return fmt.Errorf("la publicacion no trae %s", archivo.Nombre)
	}
	nuevo := filepath.Join(dir, archivo.Nombre)
	if err := a.o.Fuente.Descargar(ctx, url, nuevo, maxEjecutable); err != nil {
		return err
	}

	a.fase("verificando")
	if err := VerificarArchivo(nuevo, archivo.SHA256); err != nil {
		os.Remove(nuevo)
		return err
	}

	// Que no se corte nada a medio imprimir. Lo que siga pendiente tras la
	// espera se guarda en disco y se reintenta al arrancar.
	a.fase("esperando_cola")
	if a.o.ColaVacia != nil {
		limite := time.Now().Add(a.esperaCola)
		for !a.o.ColaVacia() && time.Now().Before(limite) {
			time.Sleep(250 * time.Millisecond)
		}
	}

	a.fase("instalando")
	o, err := a.orden(p.Version, nuevo, dir)
	if err != nil {
		return err
	}
	ruta, err := escribirOrden(dir, o)
	if err != nil {
		return err
	}
	if err := a.o.Sistema.Lanzar(ruta); err != nil {
		os.Remove(ruta)
		return fmt.Errorf("no se pudo lanzar la actualizacion: %w", err)
	}
	return nil
}

func (a *Actualizador) orden(version, nuevo, dir string) (Orden, error) {
	o := Orden{
		Version:    version,
		Plataforma: Plataforma(),
		Nuevo:      nuevo,
		Manifiesto: filepath.Join(dir, nombreManifiesto),
		Firma:      filepath.Join(dir, nombreFirma),
		Config:     a.o.ConfigPath,
		Datos:      a.o.DataDir,
		Servicio:   a.o.Servicio,
	}
	for _, r := range []*string{&o.Nuevo, &o.Manifiesto, &o.Firma, &o.Config, &o.Datos} {
		abs, err := filepath.Abs(*r)
		if err != nil {
			return o, err
		}
		*r = abs
	}
	return o, nil
}

// escribirOrden la deja completa de una vez: en Linux, la unidad .path se
// dispara en cuanto aparece orden.json, y no puede encontrarla a medias.
func escribirOrden(dir string, o Orden) (string, error) {
	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		return "", err
	}
	tmp := filepath.Join(dir, nombreOrden+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return "", err
	}
	final := filepath.Join(dir, nombreOrden)
	if err := os.Rename(tmp, final); err != nil {
		return "", err
	}
	return final, nil
}
