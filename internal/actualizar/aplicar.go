package actualizar

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"collatech-agent/internal/config"
)

type opcionesAplicar struct {
	plazo     time.Duration
	intervalo time.Duration
	// salud construye, a partir del config.json, la funcion que pregunta al
	// agente que version responde.
	salud func(configPath string) func(context.Context) (string, error)
}

// Aplicar hace el cambio que pide la orden, fuera del servicio:
//
//  1. vuelve a verificar firma, suma, plataforma y version: no se fia de lo
//     que haya en la carpeta de datos;
//  2. copia la configuracion, las impresoras y la cola;
//  3. para el servicio, cambia el ejecutable y lo arranca;
//  4. espera hasta 60 s a que responda la version nueva;
//  5. si no responde, deja todo como estaba y lo arranca otra vez.
//
// El resultado queda en resultado.json para que el panel lo ensene.
func Aplicar(ctx context.Context, ordenRuta, versionActual string, sis Sistema, claves []ed25519.PublicKey) Resultado {
	return aplicarCon(ctx, ordenRuta, versionActual, sis, claves, opcionesAplicar{})
}

func aplicarCon(ctx context.Context, ordenRuta, versionActual string, sis Sistema, claves []ed25519.PublicKey, op opcionesAplicar) Resultado {
	if op.plazo == 0 {
		op.plazo = 60 * time.Second
	}
	if op.intervalo == 0 {
		op.intervalo = 2 * time.Second
	}
	if op.salud == nil {
		op.salud = SaludLocal
	}

	dir := filepath.Dir(ordenRuta)
	res := Resultado{Desde: versionActual, Fecha: time.Now()}
	registro := abrirRegistro(dir)
	defer registro.cerrar()

	terminar := func(r Resultado) Resultado {
		r.Fecha = time.Now()
		if r.OK {
			registro.printf("actualizado de %s a %s", r.Desde, r.Hasta)
		} else {
			registro.printf("FALLO: %s", r.Error)
		}
		if b, err := json.MarshalIndent(r, "", "  "); err == nil {
			os.WriteFile(filepath.Join(dir, nombreResultado), b, 0o644)
		}
		return r
	}

	// La orden se aparta antes de nada: en Linux, la unidad .path se volveria
	// a disparar mientras orden.json exista.
	enCurso := filepath.Join(dir, "orden.en-curso.json")
	if err := os.Rename(ordenRuta, enCurso); err != nil {
		res.Error = "no hay orden de actualizacion: " + err.Error()
		return terminar(res)
	}
	defer os.Remove(enCurso)

	o, archivo, err := verificarOrden(enCurso, versionActual, claves)
	res.Hasta = o.Version
	if err != nil {
		res.Error = err.Error()
		return terminar(res)
	}
	registro.printf("orden verificada: %s -> %s (%s)", versionActual, o.Version, archivo.Nombre)

	exe, err := sis.RutaEjecutable()
	if err != nil {
		res.Error = "no se encontro el ejecutable del servicio: " + err.Error()
		return terminar(res)
	}

	copia := filepath.Join(dir, "copia")
	guardados, err := copiarDatos(o, copia)
	if err != nil {
		res.Error = "no se pudo hacer la copia de la configuracion: " + err.Error()
		return terminar(res)
	}

	registro.printf("parando el servicio")
	if err := sis.Parar(ctx); err != nil {
		res.Error = "no se pudo parar el servicio: " + err.Error()
		return terminar(res)
	}

	anterior := exe + ".anterior"
	os.Remove(anterior)
	if err := renombrarConReintentos(exe, anterior); err != nil {
		sis.Arrancar(ctx)
		res.Error = "no se pudo apartar el ejecutable actual: " + err.Error()
		return terminar(res)
	}
	if err := copiarArchivoCon(o.Nuevo, exe, 0o755); err != nil {
		os.Remove(exe)
		os.Rename(anterior, exe)
		sis.Arrancar(ctx)
		res.Error = "no se pudo poner el ejecutable nuevo: " + err.Error()
		return terminar(res)
	}

	registro.printf("arrancando la %s", o.Version)
	responde := false
	if err := sis.Arrancar(ctx); err != nil {
		registro.printf("no arranco: %v", err)
	} else {
		responde = esperarVersion(ctx, op.salud(o.Config), o.Version, op.plazo, op.intervalo, registro)
	}

	if responde {
		os.Remove(anterior)
		os.Remove(o.Nuevo)
		os.RemoveAll(copia)
		res.OK = true
		return terminar(res)
	}

	// Vuelta atras: el ejecutable y los datos como estaban.
	registro.printf("la %s no respondio: volviendo a la %s", o.Version, versionActual)
	sis.Parar(ctx)
	os.Remove(exe)
	if err := os.Rename(anterior, exe); err != nil {
		registro.printf("no se pudo restaurar el ejecutable: %v", err)
	}
	restaurarDatos(guardados, registro)
	if err := sis.Arrancar(ctx); err != nil {
		registro.printf("la version anterior no arranco: %v", err)
	}
	res.Error = fmt.Sprintf("la version %s no respondio; se volvio a la %s", o.Version, versionActual)
	return terminar(res)
}

// verificarOrden lee la orden y comprueba todo lo que el servicio ya
// comprobo, otra vez y con las claves de este ejecutable.
func verificarOrden(ruta, versionActual string, claves []ed25519.PublicKey) (Orden, Archivo, error) {
	var o Orden
	b, err := os.ReadFile(ruta)
	if err != nil {
		return o, Archivo{}, err
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return o, Archivo{}, fmt.Errorf("la orden no se puede leer: %w", err)
	}
	if o.Plataforma != Plataforma() {
		return o, Archivo{}, fmt.Errorf("la orden es para %s y esto es %s", o.Plataforma, Plataforma())
	}
	datos, err := os.ReadFile(o.Manifiesto)
	if err != nil {
		return o, Archivo{}, err
	}
	firma, err := os.ReadFile(o.Firma)
	if err != nil {
		return o, Archivo{}, err
	}
	m, err := VerificarManifiesto(datos, firma, claves)
	if err != nil {
		return o, Archivo{}, err
	}
	if m.Version != o.Version {
		return o, Archivo{}, fmt.Errorf("la orden dice %s y el manifiesto firmado %s", o.Version, m.Version)
	}
	if !EsMayor(m.Version, versionActual) {
		return o, Archivo{}, fmt.Errorf("la %s no es mas nueva que la %s instalada", m.Version, versionActual)
	}
	archivo, ok := m.Archivos[Plataforma()]
	if !ok {
		return o, Archivo{}, fmt.Errorf("el manifiesto no trae ejecutable para %s", Plataforma())
	}
	if filepath.Base(o.Nuevo) != archivo.Nombre {
		return o, Archivo{}, fmt.Errorf("la orden apunta a %s y el manifiesto a %s", filepath.Base(o.Nuevo), archivo.Nombre)
	}
	if err := VerificarArchivo(o.Nuevo, archivo.SHA256); err != nil {
		return o, Archivo{}, err
	}
	return o, archivo, nil
}

type guardado struct{ original, copia string }

// copiarDatos guarda lo que una version nueva podria reescribir al arrancar.
func copiarDatos(o Orden, destino string) ([]guardado, error) {
	os.RemoveAll(destino)
	if err := os.MkdirAll(destino, 0o700); err != nil {
		return nil, err
	}
	var out []guardado
	for i, ruta := range []string{
		o.Config,
		filepath.Join(o.Datos, "storage", "settings.json"),
		filepath.Join(o.Datos, "storage", "jobs.json"),
	} {
		info, err := os.Stat(ruta)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c := filepath.Join(destino, strconv.Itoa(i)+"-"+filepath.Base(ruta))
		if err := copiarArchivoCon(ruta, c, info.Mode().Perm()); err != nil {
			return nil, err
		}
		out = append(out, guardado{original: ruta, copia: c})
	}
	return out, nil
}

func restaurarDatos(g []guardado, r *registro) {
	for _, x := range g {
		info, err := os.Stat(x.copia)
		if err != nil {
			r.printf("no se pudo restaurar %s: %v", x.original, err)
			continue
		}
		if err := copiarArchivoCon(x.copia, x.original, info.Mode().Perm()); err != nil {
			r.printf("no se pudo restaurar %s: %v", x.original, err)
		}
	}
}

func esperarVersion(ctx context.Context, salud func(context.Context) (string, error), version string, plazo, intervalo time.Duration, r *registro) bool {
	limite := time.Now().Add(plazo)
	for {
		v, err := salud(ctx)
		if err == nil && v == version {
			return true
		}
		if time.Now().After(limite) {
			if err != nil {
				r.printf("ultima comprobacion: %v", err)
			} else {
				r.printf("ultima comprobacion: responde la %q", v)
			}
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(intervalo):
		}
	}
}

// SaludLocal pregunta al agente de esta PC, por /health, que version
// responde. Con TLS no comprueba el certificado: es el suyo propio, en
// localhost.
func SaludLocal(configPath string) func(context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		cfg, err := config.Load(configPath)
		if err != nil {
			return "", err
		}
		host := cfg.Host
		switch host {
		case "", "0.0.0.0", "::", "localhost":
			host = "127.0.0.1"
		}
		esquema := "http"
		cliente := &http.Client{Timeout: 3 * time.Second}
		if cfg.TLS.Enabled {
			esquema = "https"
			cliente.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
		}
		url := esquema + "://" + net.JoinHostPort(host, strconv.Itoa(cfg.Port)) + "/health"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		res, err := cliente.Do(req)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		var r struct {
			Data struct {
				Version string `json:"version"`
			} `json:"data"`
		}
		if err := json.NewDecoder(io.LimitReader(res.Body, 1<<16)).Decode(&r); err != nil {
			return "", err
		}
		return r.Data.Version, nil
	}
}

// renombrarConReintentos aguanta unos segundos: en Windows, un servicio
// recien parado puede tardar en soltar su .exe.
func renombrarConReintentos(de, a string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(de, a); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
}

func copiarArchivoCon(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".nuevo"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Chmod(tmp, perm)
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// registro escribe cada paso en actualizacion/aplicar.log y en la salida
// estandar (en Linux la recoge journalctl).
type registro struct {
	f *os.File
	l *log.Logger
}

func abrirRegistro(dir string) *registro {
	f, err := os.OpenFile(filepath.Join(dir, "aplicar.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	var w io.Writer = os.Stdout
	if err == nil {
		w = io.MultiWriter(os.Stdout, f)
	}
	return &registro{f: f, l: log.New(w, "actualizar: ", log.LstdFlags)}
}

func (r *registro) printf(formato string, args ...any) { r.l.Printf(formato, args...) }

func (r *registro) cerrar() {
	if r.f != nil {
		r.f.Close()
	}
}
