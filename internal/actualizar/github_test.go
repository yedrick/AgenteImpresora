package actualizar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUltima(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua = r.Header.Get("User-Agent")
		w.Write([]byte(`{"tag_name":"v1.7.0","html_url":"https://github.com/x/y/releases/tag/v1.7.0",
			"assets":[{"name":"manifiesto.json","browser_download_url":"https://d/manifiesto.json"},
			          {"name":"actualizacion-linux-amd64","browser_download_url":"https://d/bin"}]}`))
	}))
	defer srv.Close()

	f := Fuente{URLUltima: srv.URL, Cliente: srv.Client(), UserAgent: "CollaTech-Agent/1.6.4"}
	p, err := f.Ultima(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != "1.7.0" || p.URLNotas != "https://github.com/x/y/releases/tag/v1.7.0" {
		t.Fatalf("publicacion mal leida: %+v", p)
	}
	if p.Recursos["manifiesto.json"] != "https://d/manifiesto.json" || p.Recursos["actualizacion-linux-amd64"] != "https://d/bin" {
		t.Fatalf("recursos mal leidos: %+v", p.Recursos)
	}
	if ua != "CollaTech-Agent/1.6.4" {
		t.Fatalf("User-Agent = %q", ua)
	}
}

func TestUltimaErrorHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := Fuente{URLUltima: srv.URL, Cliente: srv.Client()}.Ultima(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("quiero un error con el 403, tengo %v", err)
	}
}

func TestUltimaSinRed(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nadie escucha: como un local sin internet
	_, err := Fuente{URLUltima: url, Cliente: http.DefaultClient}.Ultima(context.Background())
	if err == nil || !strings.Contains(err.Error(), "No se pudo consultar") {
		t.Fatalf("quiero \"No se pudo consultar ...\", tengo %v", err)
	}
}

func TestDescargar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("contenido"))
	}))
	defer srv.Close()
	destino := filepath.Join(t.TempDir(), "archivo")
	if err := (Fuente{Cliente: srv.Client()}).Descargar(context.Background(), srv.URL, destino, 1024); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(destino); string(b) != "contenido" {
		t.Fatalf("contenido = %q", b)
	}
}

func TestDescargarLimite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("x", 2048)))
	}))
	defer srv.Close()
	dir := t.TempDir()
	destino := filepath.Join(dir, "archivo")
	err := (Fuente{Cliente: srv.Client()}).Descargar(context.Background(), srv.URL, destino, 1024)
	if err == nil {
		t.Fatal("una descarga mayor que el limite tiene que fallar")
	}
	if entradas, _ := os.ReadDir(dir); len(entradas) != 0 {
		t.Fatalf("no tiene que quedar nada a medias, quedan %d archivos", len(entradas))
	}
}

// Un tiempo maximo global en el cliente cortaba cualquier descarga que
// tardara mas de 15 s: en un ADSL lento, el boton nunca podia actualizar.
func TestFuenteOficialSinTiempoMaximoGlobal(t *testing.T) {
	f := FuenteOficial("1.0.0")
	if f.Cliente == nil || f.Cliente.Timeout != 0 {
		t.Fatalf("el cliente no puede tener Timeout global (tiene %v): corta las descargas largas", f.Cliente.Timeout)
	}
}

func TestDescargaCortadaLoDice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		w.Write([]byte("solo un trozo"))
	}))
	defer srv.Close()
	err := (Fuente{Cliente: srv.Client()}).Descargar(context.Background(), srv.URL, filepath.Join(t.TempDir(), "x"), 1<<20)
	if err == nil || !strings.Contains(err.Error(), "se corto") {
		t.Fatalf("una descarga cortada tiene que decir que se corto, tengo %v", err)
	}
}
