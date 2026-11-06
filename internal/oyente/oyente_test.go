package oyente

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func puertoLibre(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func pedir(t *testing.T, puerto int, ruta string) (int, string) {
	t.Helper()
	c := http.Client{Timeout: 3 * time.Second}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d%s", puerto, ruta))
	if err != nil {
		t.Fatalf("GET %s: %v", ruta, err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func arrancar(t *testing.T, mux *http.ServeMux) (*Oyente, int) {
	t.Helper()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "bien") })
	srv := &http.Server{Handler: mux}
	puerto := puertoLibre(t)
	o, err := Abrir(srv, "127.0.0.1", puerto, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
	return o, puerto
}

func TestReabrirCambiaDondeEscucha(t *testing.T) {
	o, puerto := arrancar(t, http.NewServeMux())
	if c, _ := pedir(t, puerto, "/health"); c != 200 {
		t.Fatalf("antes de reabrir: %d", c)
	}
	if err := o.Reabrir("0.0.0.0"); err != nil {
		t.Fatal(err)
	}
	if o.Host() != "0.0.0.0" {
		t.Fatalf("Host() = %q, quiero 0.0.0.0", o.Host())
	}
	if c, _ := pedir(t, puerto, "/health"); c != 200 {
		t.Fatalf("en 0.0.0.0 tambien tiene que responder por 127.0.0.1: %d", c)
	}
	if err := o.Reabrir("127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if o.Host() != "127.0.0.1" {
		t.Fatalf("Host() = %q, quiero 127.0.0.1", o.Host())
	}
	if c, _ := pedir(t, puerto, "/health"); c != 200 {
		t.Fatalf("de vuelta en 127.0.0.1: %d", c)
	}
}

func TestReabrirQueFallaVuelveAlAnterior(t *testing.T) {
	o, puerto := arrancar(t, http.NewServeMux())
	// 192.0.2.1 es TEST-NET: no es de ninguna maquina, no se puede escuchar ahi.
	err := o.Reabrir("192.0.2.1")
	if err == nil || !strings.Contains(err.Error(), "sigue en 127.0.0.1") {
		t.Fatalf("quiero un error que diga donde sigue, tengo %v", err)
	}
	if o.Host() != "127.0.0.1" {
		t.Fatalf("Host() = %q tras fallar, quiero 127.0.0.1", o.Host())
	}
	if c, _ := pedir(t, puerto, "/health"); c != 200 {
		t.Fatalf("tras fallar el agente tiene que seguir respondiendo: %d", c)
	}
}

// La peticion del panel que pide el cambio viaja por el socket que se
// cierra: tiene que recibir su respuesta igual.
func TestLaPeticionQueReabreRecibeSuRespuesta(t *testing.T) {
	mux := http.NewServeMux()
	var o *Oyente
	mux.HandleFunc("/cambiar", func(w http.ResponseWriter, _ *http.Request) {
		if err := o.Reabrir("0.0.0.0"); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		io.WriteString(w, "ok")
	})
	o2, puerto := arrancar(t, mux)
	o = o2
	c, cuerpo := pedir(t, puerto, "/cambiar")
	if c != 200 || cuerpo != "ok" {
		t.Fatalf("quiero 200 ok, tengo %d %q", c, cuerpo)
	}
}
