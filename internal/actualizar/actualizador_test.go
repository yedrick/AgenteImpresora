package actualizar

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// publicacionFalsa sirve una Release con la forma de la API de GitHub,
// firmada con priv.
type publicacionFalsa struct {
	version string
	binario []byte
	minima  string
	manual  string
	// sumaFalsa publica en el manifiesto una suma que no es la del binario,
	// como si la descarga llegara cambiada.
	sumaFalsa bool
}

func (p publicacionFalsa) servir(t *testing.T, priv ed25519.PrivateKey) *httptest.Server {
	t.Helper()
	nombre := "actualizacion-" + Plataforma()
	suma := sha256.Sum256(p.binario)
	sumaHex := hex.EncodeToString(suma[:])
	if p.sumaFalsa {
		sumaHex = strings.Repeat("0", 64)
	}
	m := Manifiesto{Version: p.version, Minima: p.minima, Manual: p.manual,
		Archivos: map[string]Archivo{Plataforma(): {Nombre: nombre, SHA256: sumaHex}}}
	datos, _ := json.Marshal(m)
	firma := firmar(priv, datos)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v" + p.version,
				"html_url": srv.URL + "/notas",
				"assets": []map[string]string{
					{"name": "manifiesto.json", "browser_download_url": srv.URL + "/manifiesto.json"},
					{"name": "manifiesto.json.sig", "browser_download_url": srv.URL + "/manifiesto.json.sig"},
					{"name": nombre, "browser_download_url": srv.URL + "/bin"},
				},
			})
		case "/manifiesto.json":
			w.Write(datos)
		case "/manifiesto.json.sig":
			w.Write(firma)
		case "/bin":
			w.Write(p.binario)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// sistemaFalso apunta lo que se le pide.
type sistemaFalso struct {
	mu         sync.Mutex
	soportado  bool
	motivo     string
	exe        string
	lanzadas   []string
	paradas    int
	arranques  int
	alArrancar func() // simula lo que hace la version nueva al arrancar
	alParar    func() // simula lo que pasa justo antes de parar
}

func (s *sistemaFalso) Soportado() (bool, string)       { return s.soportado, s.motivo }
func (s *sistemaFalso) RutaEjecutable() (string, error) { return s.exe, nil }
func (s *sistemaFalso) Parar(context.Context) error {
	s.mu.Lock()
	s.paradas++
	f := s.alParar
	s.alParar = nil
	s.mu.Unlock()
	if f != nil {
		f()
	}
	return nil
}
func (s *sistemaFalso) Arrancar(context.Context) error {
	s.mu.Lock()
	s.arranques++
	f := s.alArrancar
	s.mu.Unlock()
	if f != nil {
		f()
	}
	return nil
}
func (s *sistemaFalso) Lanzar(orden string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lanzadas = append(s.lanzadas, orden)
	return nil
}
func (s *sistemaFalso) numLanzadas() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lanzadas)
}

func nuevoParaPrueba(t *testing.T, srv *httptest.Server, pub ed25519.PublicKey, sis Sistema, colaVacia func() bool) *Actualizador {
	t.Helper()
	dir := t.TempDir()
	a := NuevoActualizador(Opciones{
		Version:    "1.6.4",
		DataDir:    dir,
		ConfigPath: filepath.Join(dir, "configs", "config.json"),
		Fuente:     Fuente{URLUltima: srv.URL + "/latest", Cliente: srv.Client()},
		Sistema:    sis,
		Claves:     []ed25519.PublicKey{pub},
		ColaVacia:  colaVacia,
	})
	a.esperaCola = 300 * time.Millisecond
	return a
}

func esperarFase(t *testing.T, a *Actualizador, fases ...string) Estado {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for time.Now().Before(limite) {
		e := a.Estado()
		for _, f := range fases {
			if e.Fase == f {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no se llego a la fase %v; estado %+v", fases, a.Estado())
	return Estado{}
}

func TestBuscarNoHayNueva(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.6.4", binario: []byte("x")}.servir(t, priv)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: true}, nil)
	d, err := a.Buscar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.VersionActual != "1.6.4" || d.Nueva != nil {
		t.Fatalf("con la misma version no hay nada nuevo: %+v", d)
	}
}

func TestBuscarHayNueva(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("x")}.servir(t, priv)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: true}, nil)
	d, err := a.Buscar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Nueva == nil || d.Nueva.Version != "1.7.0" || d.Nueva.URLNotas != srv.URL+"/notas" || d.Nueva.ManualMotivo != "" {
		t.Fatalf("tiene que ofrecer la 1.7.0 instalable: %+v", d.Nueva)
	}
}

func TestBuscarManualPorMinima(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.8.0", binario: []byte("x"), minima: "1.7.0"}.servir(t, priv)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: true}, nil)
	d, err := a.Buscar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if d.Nueva == nil || !strings.Contains(d.Nueva.ManualMotivo, "1.7.0") {
		t.Fatalf("desde la 1.6.4, con minima 1.7.0, hay que instalar a mano: %+v", d.Nueva)
	}
	if err := a.Instalar("1.8.0"); err == nil {
		t.Fatal("una version que se instala a mano no se puede instalar con el boton")
	}
}

func TestBuscarManualPorTexto(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("x"), manual: "cambia el firewall"}.servir(t, priv)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: true}, nil)
	d, _ := a.Buscar(context.Background())
	if d.Nueva == nil || d.Nueva.Manual != "cambia el firewall" || d.Nueva.ManualMotivo == "" {
		t.Fatalf("el texto manual del manifiesto tiene que llegar al panel: %+v", d.Nueva)
	}
}

func TestBuscarFirmaDeOtraClave(t *testing.T) {
	pub, _ := clavePrueba(t)
	_, otra := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("x")}.servir(t, otra)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: true}, nil)
	if _, err := a.Buscar(context.Background()); err == nil || !strings.Contains(err.Error(), "firma") {
		t.Fatalf("una publicacion firmada con otra clave tiene que rechazarse, tengo %v", err)
	}
}

func TestInstalarSinBuscar(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("x")}.servir(t, priv)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: true}, nil)
	if err := a.Instalar("1.7.0"); err == nil {
		t.Fatal("hay que buscar antes de instalar")
	}
}

func TestInstalarNoSoportado(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("x")}.servir(t, priv)
	a := nuevoParaPrueba(t, srv, pub, &sistemaFalso{soportado: false, motivo: "en Mac se actualiza a mano"}, nil)
	a.Buscar(context.Background())
	if err := a.Instalar("1.7.0"); err == nil || !strings.Contains(err.Error(), "Mac") {
		t.Fatalf("tiene que devolver el motivo, tengo %v", err)
	}
}

func TestInstalarArchivoCorrupto(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("binario nuevo"), sumaFalsa: true}.servir(t, priv)
	sis := &sistemaFalso{soportado: true}
	a := nuevoParaPrueba(t, srv, pub, sis, nil)
	if _, err := a.Buscar(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.Instalar("1.7.0"); err != nil {
		t.Fatal(err)
	}
	e := esperarFase(t, a, "error")
	if !strings.Contains(e.Error, "no coincide") {
		t.Fatalf("el error tiene que decir que no coincide: %q", e.Error)
	}
	if sis.numLanzadas() != 0 {
		t.Fatal("con el archivo mal no se puede lanzar nada")
	}
}

func TestInstalarSoloUnaVez(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("binario nuevo")}.servir(t, priv)
	sis := &sistemaFalso{soportado: true}
	// La cola nunca se vacia: la primera instalacion se queda esperando.
	a := nuevoParaPrueba(t, srv, pub, sis, func() bool { return false })
	a.Buscar(context.Background())
	if err := a.Instalar("1.7.0"); err != nil {
		t.Fatal(err)
	}
	err := a.Instalar("1.7.0")
	if err == nil || !strings.Contains(err.Error(), "en curso") {
		t.Fatalf("la segunda tiene que rechazarse por estar una en curso, tengo %v", err)
	}
	esperarFase(t, a, "instalando", "error")
}

func TestInstalarLanzaOrden(t *testing.T) {
	pub, priv := clavePrueba(t)
	binario := []byte("binario nuevo")
	srv := publicacionFalsa{version: "1.7.0", binario: binario}.servir(t, priv)
	sis := &sistemaFalso{soportado: true}
	a := nuevoParaPrueba(t, srv, pub, sis, func() bool { return true })
	a.Buscar(context.Background())
	if err := a.Instalar("1.7.0"); err != nil {
		t.Fatal(err)
	}
	esperarFase(t, a, "instalando")
	limite := time.Now().Add(3 * time.Second)
	for sis.numLanzadas() == 0 && time.Now().Before(limite) {
		time.Sleep(10 * time.Millisecond)
	}
	if sis.numLanzadas() != 1 {
		t.Fatalf("se tiene que lanzar una orden, se lanzaron %d", sis.numLanzadas())
	}
	b, err := os.ReadFile(sis.lanzadas[0])
	if err != nil {
		t.Fatal(err)
	}
	var o Orden
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatal(err)
	}
	if o.Version != "1.7.0" || o.Plataforma != Plataforma() {
		t.Fatalf("orden mal: %+v", o)
	}
	for _, ruta := range []string{o.Nuevo, o.Manifiesto, o.Firma, o.Config, o.Datos} {
		if !filepath.IsAbs(ruta) {
			t.Fatalf("las rutas de la orden tienen que ser absolutas: %+v", o)
		}
	}
	if got, _ := os.ReadFile(o.Nuevo); string(got) != string(binario) {
		t.Fatal("el ejecutable descargado no es el de la publicacion")
	}
}

// Si el aplicador falla antes de reiniciar el servicio, este proceso sigue
// vivo en "instalando": tiene que enterarse por resultado.json.
func TestEstadoTomaElResultadoDelAplicador(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("binario nuevo")}.servir(t, priv)
	sis := &sistemaFalso{soportado: true}
	a := nuevoParaPrueba(t, srv, pub, sis, func() bool { return true })
	a.Buscar(context.Background())
	if err := a.Instalar("1.7.0"); err != nil {
		t.Fatal(err)
	}
	esperarFase(t, a, "instalando")
	time.Sleep(20 * time.Millisecond)
	r := Resultado{Desde: "1.6.4", Hasta: "1.7.0", Error: "no se pudo parar el servicio", Fecha: time.Now()}
	b, _ := json.Marshal(r)
	os.WriteFile(filepath.Join(a.carpeta(), nombreResultado), b, 0o644)

	e := a.Estado()
	if e.Fase != "error" || !strings.Contains(e.Error, "no se pudo parar el servicio") {
		t.Fatalf("tiene que pasar a error con el motivo del aplicador: %+v", e)
	}
	if err := a.Instalar("1.7.0"); err != nil && strings.Contains(err.Error(), "en curso") {
		t.Fatal("tras el fallo se tiene que poder volver a intentar")
	}
}

// Si nadie recoge la orden (la pieza de systemd no esta activa), no se puede
// quedar en "instalando" para siempre ni dejar la orden para el proximo
// arranque.
func TestEstadoInstalandoCaduca(t *testing.T) {
	pub, priv := clavePrueba(t)
	srv := publicacionFalsa{version: "1.7.0", binario: []byte("binario nuevo")}.servir(t, priv)
	sis := &sistemaFalso{soportado: true}
	a := nuevoParaPrueba(t, srv, pub, sis, func() bool { return true })
	a.plazoInstalando = 200 * time.Millisecond
	a.Buscar(context.Background())
	a.Instalar("1.7.0")
	esperarFase(t, a, "instalando")
	time.Sleep(300 * time.Millisecond)
	e := a.Estado()
	if e.Fase != "error" || !strings.Contains(e.Error, "no arranco") {
		t.Fatalf("tiene que caducar con un error claro: %+v", e)
	}
	if _, err := os.Stat(filepath.Join(a.carpeta(), nombreOrden)); !os.IsNotExist(err) {
		t.Fatal("la orden sin recoger tiene que borrarse")
	}
}
