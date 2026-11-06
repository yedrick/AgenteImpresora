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
	"strings"
	"time"
	"unicode"

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
// En Linux el servicio esta encerrado y este proceso corre como root sin
// encierro, asi que nada de la orden decide donde se escribe: las rutas
// salen de los argumentos (configPath) y de donde esta la orden, lo que se
// instala es una copia privada de lo verificado, y no se siguen enlaces.
//
// El resultado queda en resultado.json para que el panel lo ensene.
func Aplicar(ctx context.Context, ordenRuta, configPath, versionActual string, sis Sistema, claves []ed25519.PublicKey) Resultado {
	return aplicarCon(ctx, ordenRuta, configPath, versionActual, sis, claves, opcionesAplicar{})
}

func aplicarCon(ctx context.Context, ordenRuta, configPath, versionActual string, sis Sistema, claves []ed25519.PublicKey, op opcionesAplicar) Resultado {
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
	datos := filepath.Dir(dir)
	res := Resultado{Desde: versionActual, Fecha: time.Now()}

	// Si la carpeta de la actualizacion (o la de datos) fuese un enlace,
	// todo lo de abajo escribiria donde apunte. No se toca nada.
	for _, d := range []string{datos, dir} {
		if err := esCarpetaReal(d); err != nil {
			res.Error = err.Error()
			return res
		}
	}

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
			if err := escribirNuevo(filepath.Join(dir, nombreResultado), b, 0o644); err != nil {
				registro.printf("no se pudo escribir el resultado: %v", err)
			}
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

	// Lo verificado se copia a una carpeta que solo puede leer quien ejecuta
	// esto: el servicio no puede cambiarlo entre la comprobacion y la copia.
	privado, err := os.MkdirTemp("", "collatech-aplicar")
	if err != nil {
		res.Error = "no se pudo preparar la carpeta de trabajo: " + err.Error()
		return terminar(res)
	}
	defer os.RemoveAll(privado)

	o, verificado, err := verificarOrden(enCurso, dir, privado, versionActual, claves)
	res.Hasta = o.Version
	if err != nil {
		res.Error = err.Error()
		return terminar(res)
	}
	registro.printf("orden verificada: %s -> %s", versionActual, o.Version)

	exe, err := sis.RutaEjecutable()
	if err != nil {
		res.Error = "no se encontro el ejecutable del servicio: " + err.Error()
		return terminar(res)
	}

	guardados, err := copiarDatos(configPath, datos, filepath.Join(privado, "copia"))
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
	if err := copiarArchivoCon(verificado, exe, 0o755); err != nil {
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
		responde = esperarVersion(ctx, op.salud(configPath), o.Version, op.plazo, op.intervalo, registro)
	}

	nuevo := filepath.Join(dir, filepath.Base(o.Nuevo))
	if responde {
		os.Remove(anterior)
		os.Remove(nuevo)
		res.OK = true
		return terminar(res)
	}

	// Vuelta atras: el ejecutable y los datos como estaban.
	registro.printf("la %s no respondio: volviendo a la %s", o.Version, versionActual)
	sis.Parar(ctx)
	var fallos []string
	os.Remove(exe)
	if err := renombrarConReintentos(anterior, exe); err != nil {
		fallos = append(fallos, "ejecutable: "+err.Error())
	}
	fallos = append(fallos, restaurarDatos(guardados, registro)...)
	if err := sis.Arrancar(ctx); err != nil {
		fallos = append(fallos, "arrancar: "+err.Error())
	}
	os.Remove(nuevo)
	if len(fallos) > 0 {
		res.Error = fmt.Sprintf("la version %s no respondio y NO se pudo volver a la %s (%s). Reinstala el agente con el instalador o el paquete.",
			o.Version, versionActual, strings.Join(fallos, "; "))
	} else {
		res.Error = fmt.Sprintf("la version %s no respondio; se volvio a la %s", o.Version, versionActual)
	}
	return terminar(res)
}

// verificarOrden lee la orden y comprueba todo lo que el servicio ya
// comprobo, otra vez y con las claves de este ejecutable. El manifiesto y la
// firma se leen de la carpeta de la orden, no de donde diga la orden.
// Devuelve la ruta de una copia privada del ejecutable verificado.
func verificarOrden(ruta, dir, privado, versionActual string, claves []ed25519.PublicKey) (Orden, string, error) {
	var o Orden
	b, err := os.ReadFile(ruta)
	if err != nil {
		return o, "", err
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return o, "", fmt.Errorf("la orden no se puede leer: %w", err)
	}
	o.Version, o.Plataforma = limpiar(o.Version), limpiar(o.Plataforma)
	if o.Plataforma != Plataforma() {
		return o, "", fmt.Errorf("la orden es para %s y esto es %s", o.Plataforma, Plataforma())
	}
	datos, err := leerSinEnlaces(filepath.Join(dir, nombreManifiesto))
	if err != nil {
		return o, "", err
	}
	firma, err := leerSinEnlaces(filepath.Join(dir, nombreFirma))
	if err != nil {
		return o, "", err
	}
	m, err := VerificarManifiesto(datos, firma, claves)
	if err != nil {
		return o, "", err
	}
	if m.Version != o.Version {
		return o, "", fmt.Errorf("la orden dice %s y el manifiesto firmado %s", o.Version, m.Version)
	}
	if !EsMayor(m.Version, versionActual) {
		return o, "", fmt.Errorf("la %s no es mas nueva que la %s instalada", m.Version, versionActual)
	}
	if m.Manual != "" || (m.Minima != "" && Comparar(versionActual, m.Minima) < 0) {
		return o, "", fmt.Errorf("la %s se instala a mano, no con el boton", m.Version)
	}
	archivo, ok := m.Archivos[Plataforma()]
	if !ok {
		return o, "", fmt.Errorf("el manifiesto no trae ejecutable para %s", Plataforma())
	}
	if archivo.Nombre != filepath.Base(archivo.Nombre) {
		return o, "", fmt.Errorf("el manifiesto trae un nombre de archivo no valido")
	}
	esperado := filepath.Join(dir, archivo.Nombre)
	if filepath.Clean(o.Nuevo) != esperado {
		return o, "", fmt.Errorf("la orden apunta a otro ejecutable que el de la publicacion")
	}
	verificado := filepath.Join(privado, "nuevo")
	if err := copiarSinEnlaces(esperado, verificado); err != nil {
		return o, "", err
	}
	if err := VerificarArchivo(verificado, archivo.SHA256); err != nil {
		return o, "", err
	}
	return o, verificado, nil
}

type guardado struct{ original, copia string }

// copiarDatos guarda lo que una version nueva podria reescribir al arrancar.
// Solo archivos normales: un enlace se salta.
func copiarDatos(configPath, datos, destino string) ([]guardado, error) {
	if err := os.MkdirAll(destino, 0o700); err != nil {
		return nil, err
	}
	var out []guardado
	for i, ruta := range []string{
		configPath,
		filepath.Join(datos, "storage", "settings.json"),
		filepath.Join(datos, "storage", "jobs.json"),
	} {
		if ruta == "" {
			continue
		}
		info, err := os.Lstat(ruta)
		if errors.Is(err, os.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
			continue
		}
		if err != nil {
			return nil, err
		}
		c := filepath.Join(destino, strconv.Itoa(i)+"-"+filepath.Base(ruta))
		if err := copiarSinEnlaces(ruta, c); err != nil {
			return nil, err
		}
		os.Chmod(c, info.Mode().Perm())
		out = append(out, guardado{original: ruta, copia: c})
	}
	return out, nil
}

func restaurarDatos(g []guardado, r *registro) []string {
	var fallos []string
	for _, x := range g {
		err := esCarpetaReal(filepath.Dir(x.original))
		var info os.FileInfo
		if err == nil {
			info, err = os.Stat(x.copia)
		}
		if err == nil {
			err = copiarArchivoCon(x.copia, x.original, info.Mode().Perm())
		}
		if err != nil {
			r.printf("no se pudo restaurar %s: %v", x.original, err)
			fallos = append(fallos, filepath.Base(x.original)+": "+err.Error())
		}
	}
	return fallos
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

// esCarpetaReal falla si la ruta no es una carpeta o es un enlace.
func esCarpetaReal(d string) error {
	info, err := os.Lstat(d)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s no es una carpeta normal: no se toca nada", d)
	}
	return nil
}

// leerSinEnlaces lee un archivo que tiene que ser normal, no un enlace.
func leerSinEnlaces(ruta string) ([]byte, error) {
	info, err := os.Lstat(ruta)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s no es un archivo normal", filepath.Base(ruta))
	}
	return os.ReadFile(ruta)
}

// copiarSinEnlaces copia src (que tiene que ser un archivo normal) a dst,
// creando dst de cero.
func copiarSinEnlaces(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s no es un archivo normal", filepath.Base(src))
	}
	return copiarArchivoCon(src, dst, 0o600)
}

// renombrarConReintentos aguanta unos segundos: en Windows, un servicio
// recien parado puede tardar en soltar su .exe.
func renombrarConReintentos(de, a string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(de, a); err == nil {
			return nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
}

// copiarArchivoCon escribe en dst.nuevo, creado de cero (O_EXCL no sigue un
// enlace que alguien haya dejado con ese nombre), y lo renombra a dst
// (renombrar sustituye un enlace, no escribe a traves de el).
func copiarArchivoCon(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".nuevo"
	os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
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

// escribirNuevo escribe datos en ruta sin seguir un enlace que hubiera ahi.
func escribirNuevo(ruta string, datos []byte, perm os.FileMode) error {
	tmp := ruta + ".nuevo"
	os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(datos); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, ruta)
}

// limpiar quita caracteres de control de un texto que viene de la orden,
// para que no pueda inventar lineas en el registro.
func limpiar(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// registro escribe cada paso en actualizacion/aplicar.log (nuevo en cada
// ejecucion, sin seguir enlaces) y en la salida estandar (en Linux la recoge
// journalctl).
type registro struct {
	f *os.File
	l *log.Logger
}

func abrirRegistro(dir string) *registro {
	ruta := filepath.Join(dir, "aplicar.log")
	os.Remove(ruta)
	f, err := os.OpenFile(ruta, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	var w io.Writer = os.Stdout
	if err == nil {
		w = io.MultiWriter(os.Stdout, f)
	}
	return &registro{f: f, l: log.New(w, "actualizar: ", log.LstdFlags)}
}

func (r *registro) printf(formato string, args ...any) {
	r.l.Print(limpiar(fmt.Sprintf(formato, args...)))
}

func (r *registro) cerrar() {
	if r.f != nil {
		r.f.Close()
	}
}
