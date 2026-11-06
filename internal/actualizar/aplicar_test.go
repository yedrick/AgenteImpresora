package actualizar

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// escenario prepara una instalacion falsa: el ejecutable viejo, su
// configuracion y una orden con la version nueva firmada con priv.
type escenario struct {
	datos, exe, config, orden, nuevo string
	sis                              *sistemaFalso
	pub                              ed25519.PublicKey
}

func prepararEscenario(t *testing.T, versionNueva string) escenario {
	t.Helper()
	pub, priv := clavePrueba(t)
	datos := t.TempDir()
	dir := filepath.Join(datos, "actualizacion")
	os.MkdirAll(dir, 0o755)
	os.MkdirAll(filepath.Join(datos, "storage"), 0o755)
	os.MkdirAll(filepath.Join(datos, "configs"), 0o755)

	exe := filepath.Join(datos, "bin", "collatech-agent")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("VIEJO"), 0o755)
	config := filepath.Join(datos, "configs", "config.json")
	os.WriteFile(config, []byte(`{"auth_token":"original"}`), 0o600)
	os.WriteFile(filepath.Join(datos, "storage", "settings.json"), []byte(`{"impresoras":"original"}`), 0o644)

	nombre := "actualizacion-" + Plataforma()
	nuevo := filepath.Join(dir, nombre)
	os.WriteFile(nuevo, []byte("NUEVO"), 0o644)
	suma := sha256.Sum256([]byte("NUEVO"))
	m := Manifiesto{Version: versionNueva, Archivos: map[string]Archivo{Plataforma(): {Nombre: nombre, SHA256: hex.EncodeToString(suma[:])}}}
	b, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(dir, nombreManifiesto), b, 0o644)
	os.WriteFile(filepath.Join(dir, nombreFirma), firmar(priv, b), 0o644)

	o := Orden{Version: versionNueva, Plataforma: Plataforma(), Nuevo: nuevo,
		Manifiesto: filepath.Join(dir, nombreManifiesto), Firma: filepath.Join(dir, nombreFirma),
		Config: config, Datos: datos, Servicio: "prueba"}
	ruta, err := escribirOrden(dir, o)
	if err != nil {
		t.Fatal(err)
	}
	return escenario{datos: datos, exe: exe, config: config, orden: ruta, nuevo: nuevo,
		sis: &sistemaFalso{soportado: true, exe: exe}, pub: pub}
}

func (e escenario) aplicar(salud func(context.Context) (string, error)) Resultado {
	return aplicarCon(context.Background(), e.orden, "1.6.4", e.sis, []ed25519.PublicKey{e.pub}, opcionesAplicar{
		plazo:     300 * time.Millisecond,
		intervalo: 20 * time.Millisecond,
		salud:     func(string) func(context.Context) (string, error) { return salud },
	})
}

func leer(t *testing.T, ruta string) string {
	t.Helper()
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatalf("leer %s: %v", ruta, err)
	}
	return string(b)
}

func resultadoEscrito(t *testing.T, e escenario) Resultado {
	t.Helper()
	var r Resultado
	if err := json.Unmarshal([]byte(leer(t, filepath.Join(e.datos, "actualizacion", nombreResultado))), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAplicarExito(t *testing.T) {
	e := prepararEscenario(t, "1.7.0")
	r := e.aplicar(func(context.Context) (string, error) { return "1.7.0", nil })
	if !r.OK || r.Desde != "1.6.4" || r.Hasta != "1.7.0" {
		t.Fatalf("resultado: %+v", r)
	}
	if leer(t, e.exe) != "NUEVO" {
		t.Fatal("el ejecutable tiene que ser el nuevo")
	}
	if info, _ := os.Stat(e.exe); info.Mode().Perm()&0o100 == 0 {
		t.Fatal("el ejecutable nuevo tiene que poder ejecutarse")
	}
	if _, err := os.Stat(e.exe + ".anterior"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("tras el exito no tiene que quedar el .anterior")
	}
	if _, err := os.Stat(filepath.Join(e.datos, "actualizacion", "orden.en-curso.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("la orden tiene que borrarse al terminar")
	}
	if !resultadoEscrito(t, e).OK {
		t.Fatal("resultado.json tiene que decir ok")
	}
	if leer(t, e.config) != `{"auth_token":"original"}` {
		t.Fatal("la configuracion no se toca")
	}
}

func TestAplicarVuelveAtrasSiNoResponde(t *testing.T) {
	e := prepararEscenario(t, "1.7.0")
	arranques := 0
	e.sis.alArrancar = func() {
		arranques++
		if arranques == 1 { // la version nueva arranca y estropea los datos
			os.WriteFile(e.config, []byte(`{"auth_token":"cambiado"}`), 0o600)
			os.WriteFile(filepath.Join(e.datos, "storage", "settings.json"), []byte(`{"impresoras":"cambiado"}`), 0o644)
		}
	}
	r := e.aplicar(func(context.Context) (string, error) { return "", errors.New("connection refused") })
	if r.OK || !strings.Contains(r.Error, "1.7.0") || !strings.Contains(r.Error, "1.6.4") {
		t.Fatalf("tiene que fallar diciendo que se volvio de la 1.7.0 a la 1.6.4: %+v", r)
	}
	if leer(t, e.exe) != "VIEJO" {
		t.Fatal("el ejecutable tiene que volver al viejo")
	}
	if leer(t, e.config) != `{"auth_token":"original"}` {
		t.Fatal("la configuracion tiene que volver a la de antes: si no, el token cambia")
	}
	if leer(t, filepath.Join(e.datos, "storage", "settings.json")) != `{"impresoras":"original"}` {
		t.Fatal("las impresoras tienen que volver a las de antes")
	}
	if e.sis.arranques != 2 || e.sis.paradas != 2 {
		t.Fatalf("parar, arrancar nueva, parar, arrancar vieja: paradas=%d arranques=%d", e.sis.paradas, e.sis.arranques)
	}
	if resultadoEscrito(t, e).OK {
		t.Fatal("resultado.json tiene que decir que fallo")
	}
}

func TestAplicarVuelveAtrasSiVersionDistinta(t *testing.T) {
	e := prepararEscenario(t, "1.7.0")
	r := e.aplicar(func(context.Context) (string, error) { return "1.6.4", nil })
	if r.OK || leer(t, e.exe) != "VIEJO" {
		t.Fatalf("si responde otra version, se vuelve atras: %+v", r)
	}
}

func TestAplicarRechazaOrdenManipulada(t *testing.T) {
	e := prepararEscenario(t, "1.7.0")
	os.WriteFile(e.nuevo, []byte("OTRA COSA"), 0o644) // alguien cambia el ejecutable descargado
	r := e.aplicar(func(context.Context) (string, error) { return "1.7.0", nil })
	if r.OK || !strings.Contains(r.Error, "no coincide") {
		t.Fatalf("tiene que rechazar el ejecutable cambiado: %+v", r)
	}
	if e.sis.paradas != 0 || leer(t, e.exe) != "VIEJO" {
		t.Fatal("con la orden manipulada no se para ni se toca nada")
	}
}

func TestAplicarRechazaVersionNoMayor(t *testing.T) {
	e := prepararEscenario(t, "1.6.4")
	r := e.aplicar(func(context.Context) (string, error) { return "1.6.4", nil })
	if r.OK || e.sis.paradas != 0 {
		t.Fatalf("no se instala una version que no es mayor que la actual: %+v", r)
	}
}

func TestAplicarSinOrden(t *testing.T) {
	r := aplicarCon(context.Background(), filepath.Join(t.TempDir(), "orden.json"), "1.6.4", &sistemaFalso{}, nil, opcionesAplicar{})
	if r.OK || r.Error == "" {
		t.Fatalf("sin orden tiene que fallar: %+v", r)
	}
}
