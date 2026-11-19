package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"collatech-agent/internal/config"
)

// pedirConHost hace un GET con la cabecera Host que se le diga.
func pedirConHost(h http.Handler, path, remote, host, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remote
	req.Host = host
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Una web cualquiera puede hacer que su dominio apunte a 127.0.0.1 (DNS
// rebinding). El navegador la trata como su propio origen, la peticion llega
// por el bucle local y, sin mirar el Host, el agente la tomaba por el
// operador: le daba el token y las rutas de administracion.
func TestHostAjenoNoEntraComoLocal(t *testing.T) {
	h := newTestServer(t, nil)

	for _, ruta := range []string{"/api/token", "/api/status", "/api/cors"} {
		rec := pedirConHost(h, ruta, "127.0.0.1:5000", "sitio-atacante.example:18743", "")
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s con Host ajeno: %d, tenia que ser 403", ruta, rec.Code)
		}
	}
}

func TestHostsPropiosSiguenEntrando(t *testing.T) {
	h := newTestServer(t, nil)
	propio, _ := os.Hostname()

	hosts := []string{
		"localhost:18743", "LOCALHOST", "127.0.0.1:18743", "[::1]:18743",
		"192.168.1.10:18743", "192.168.1.10", "",
	}
	if propio != "" {
		hosts = append(hosts, propio+":18743", strings.ToUpper(propio), propio+".local:18743")
	}
	for _, host := range hosts {
		if rec := pedirConHost(h, "/api/token", "127.0.0.1:5000", host, ""); rec.Code != http.StatusOK {
			t.Errorf("Host %q: %d, tenia que ser 200: %s", host, rec.Code, rec.Body)
		}
	}
}

// Quien publica el agente con un nombre propio lo declara en allowed_hosts.
func TestAllowedHosts(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowedHosts = []string{"Agente.MiTienda.com"}
	})
	if rec := pedirConHost(h, "/api/token", "127.0.0.1:5000", "agente.mitienda.com:18743", ""); rec.Code != http.StatusOK {
		t.Fatalf("un host de allowed_hosts dio %d: %s", rec.Code, rec.Body)
	}
	if rec := pedirConHost(h, "/api/token", "127.0.0.1:5000", "otra.mitienda.com:18743", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("un host fuera de allowed_hosts dio %d", rec.Code)
	}
}

// Desde la red lo que protege es el token: alli el nombre con el que se
// llega al agente (el que le ponga el router, por ejemplo) no se toca.
func TestHostNoAfectaALaRedConToken(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "secreto"
	})
	if rec := pedirConHost(h, "/api/status", "192.168.1.30:5000", "caja-principal.lan:18743", "secreto"); rec.Code != http.StatusOK {
		t.Fatalf("desde la red con token dio %d: %s", rec.Code, rec.Body)
	}
}

// Una IP de confianza entra sin token, asi que el rebinding tambien le
// alcanza: el navegador de esa maquina haria la peticion por ella.
func TestHostAjenoDesdeIPDeConfianza(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "secreto"
		c.TrustedIPs = []string{"192.168.1.30"}
	})
	if rec := pedirConHost(h, "/api/status", "192.168.1.30:5000", "sitio-atacante.example:18743", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("IP de confianza con Host ajeno dio %d, tenia que ser 403", rec.Code)
	}
	if rec := pedirConHost(h, "/api/status", "192.168.1.30:5000", "192.168.1.10:18743", ""); rec.Code != http.StatusOK {
		t.Fatalf("IP de confianza con la IP del agente dio %d", rec.Code)
	}
}

// El agente acepta el token en X-CollaTech-Token y asi lo dice su mensaje de
// error, pero el preflight no la anunciaba: desde un navegador nunca llegaba.
func TestPreflightAnunciaXCollaTechToken(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowedCORS = []string{"https://mi-tienda.com"}
	})
	req := httptest.NewRequest(http.MethodOptions, "/api/print/text", nil)
	req.Header.Set("Origin", "https://mi-tienda.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "x-collatech-token,content-type")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	permitidas := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	if !strings.Contains(permitidas, "x-collatech-token") {
		t.Fatalf("el preflight no anuncia X-CollaTech-Token: %q", permitidas)
	}
}

// --- Destinos directos --------------------------------------------------------

// Un nombre que no esta dado de alta se usaba tal cual como destino. Con
// device:// eso era escribir en cualquier archivo que el agente pudiera
// abrir (y corre como root): config.json, un disco entero en /dev/sda...
func TestImprimirNoEscribeEnArchivosSinDarDeAlta(t *testing.T) {
	h := newTestServer(t, nil)
	victima := filepath.Join(t.TempDir(), "victima.txt")
	if err := os.WriteFile(victima, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, destino := range []string{"device://" + victima, "device://./storage/settings.json"} {
		rec := post(t, h, "/api/print/raw", `{"printer":"`+destino+`","data":"INYECTADO"}`, "")
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, tenia que ser 400: %s", destino, rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), "dala de alta") {
			t.Errorf("%s: el error no dice como arreglarlo: %s", destino, rec.Body)
		}
	}
	time.Sleep(200 * time.Millisecond) // por si algo llego a encolarse
	if b, _ := os.ReadFile(victima); string(b) != "original" {
		t.Fatalf("el archivo cambio: %q", b)
	}
}

func TestDestinosDirectosPeligrososSeRechazan(t *testing.T) {
	h := newTestServer(t, nil)
	for _, destino := range []string{
		"/dev/sda", "/dev/mem", "device:///dev/sda1", "device:///etc/passwd",
		"/dev/usb/../sda", "com://../etc/passwd", "bt://sda",
		// Servicios de la propia PC del agente, a los que la red no llega.
		"tcp://127.0.0.1:6379", "127.0.0.1:6379", "localhost:6379", "tcp://localhost",
		"tcp://[::1]:80", "tcp://0.0.0.0:80",
	} {
		for _, ruta := range []string{"/api/print/raw", "/api/print/text"} {
			rec := post(t, h, ruta, `{"printer":"`+destino+`","data":"x","text":"x"}`, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s a %s: %d, tenia que ser 400", ruta, destino, rec.Code)
			}
		}
	}
	// La consulta de estado tampoco abre ni llama a esos destinos.
	rec := get(t, h, "/api/stations/localhost:6379", "", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("estado de localhost:6379: %d, tenia que ser 400", rec.Code)
	}
}

// Lo que ya funcionaba sin dar de alta tiene que seguir funcionando: una
// impresora de red, un puerto de impresora y una cola del sistema.
func TestDestinosDirectosDeImpresoraSiguenValiendo(t *testing.T) {
	h := newTestServer(t, nil)
	for _, destino := range []string{
		"tcp://192.168.1.50:9100", "192.168.1.50:9100", "tcp://impresora-cocina",
		"/dev/usb/lp0", "device:///dev/usb/lp0", "/dev/ttyUSB0", "bt://rfcomm0",
		"EPSON Caja", "printer://EPSON Caja",
	} {
		rec := post(t, h, "/api/print/text", `{"printer":"`+destino+`","text":"hola"}`, "")
		if rec.Code != http.StatusAccepted && rec.Code != http.StatusOK {
			t.Errorf("%s: %d, tenia que aceptarse: %s", destino, rec.Code, rec.Body)
		}
	}
}

// Dar de alta desde el panel (solo en la PC del agente) es lo que autoriza
// un destino fuera de lo habitual, como el archivo con el que se prueba sin
// impresora.
func TestDestinoDadoDeAltaSiSeUsa(t *testing.T) {
	h := newTestServer(t, nil)
	rollo := filepath.Join(t.TempDir(), "rollo.bin")
	if err := os.WriteFile(rollo, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	alta := `[{"name":"pruebas","target":"device://` + rollo + `"}]`
	if rec := post(t, h, "/api/printers-config", alta, ""); rec.Code != http.StatusOK {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body)
	}

	// Por su nombre y tambien por el destino tal cual, que ya esta autorizado.
	for _, printer := range []string{"pruebas", "device://" + rollo} {
		rec := post(t, h, "/api/print/raw", `{"printer":"`+printer+`","data":"HOLA"}`, "")
		if rec.Code != http.StatusAccepted && rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", printer, rec.Code, rec.Body)
		}
	}
	for i := 0; i < 50; i++ {
		if b, _ := os.ReadFile(rollo); string(b) == "HOLAHOLA" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(rollo)
	t.Fatalf("el rollo dado de alta no recibio los dos trabajos: %q", b)
}
