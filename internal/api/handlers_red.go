// Acceso desde la red: el interruptor del panel y su estado.
package api

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"rsc.io/qr"

	"collatech-agent/internal/config"
)

// Conexion es un aparato que abrio /conectar desde la red.
type Conexion struct {
	IP   string    `json:"ip"`
	Hora time.Time `json:"hora"`
}

// maxConexiones acota la lista: es para ver la prueba, no un historial.
const maxConexiones = 10

// Donde escucha el agente segun el interruptor.
const (
	hostRed      = "0.0.0.0"
	hostSoloEsta = "127.0.0.1"
)

// red devuelve el estado de red vivo.
func (s *Server) red() (host string, remoto bool, token string) {
	s.redMu.RLock()
	defer s.redMu.RUnlock()
	return s.cfg.Host, s.cfg.AllowRemote, s.cfg.AuthToken
}

// cfgViva es una copia de la configuracion con el estado de red vivo.
func (s *Server) cfgViva() config.Config {
	s.redMu.RLock()
	defer s.redMu.RUnlock()
	return s.cfg
}

// comandoCortafuegos es lo que el panel ofrece copiar si otro aparato no
// llega. El agente no lo ejecuta: no toca el cortafuegos del sistema.
func comandoCortafuegos(goos string, puerto int) string {
	p := strconv.Itoa(puerto)
	switch goos {
	case "windows":
		return `netsh advfirewall firewall add rule name="CollaTech Agent ` + p +
			`" dir=in action=allow protocol=TCP localport=` + p
	case "darwin":
		return "El cortafuegos de macOS pregunta la primera vez que el agente escucha en la red; acepta ahi."
	default:
		return "sudo ufw allow " + p + "/tcp"
	}
}

func (s *Server) puerto() int {
	if s.cfg.Port == 0 {
		return config.DefaultPort
	}
	return s.cfg.Port
}

func (s *Server) estadoRed() map[string]any {
	_, remoto, token := s.red()
	esquema := "http"
	if s.cfg.TLS.Enabled {
		esquema = "https"
	}
	urls := []string{}
	for _, ip := range localIPv4s() {
		urls = append(urls, fmt.Sprintf("%s://%s:%d/conectar", esquema, ip, s.puerto()))
	}
	s.redMu.RLock()
	conexiones := append([]Conexion{}, s.conexiones...)
	s.redMu.RUnlock()
	return map[string]any{
		"conexiones":      conexiones,
		"activo":          remoto,
		"puerto":          s.puerto(),
		"token_requerido": remoto && token != "",
		"urls_prueba":     urls,
		"cortafuegos":     comandoCortafuegos(runtime.GOOS, s.puerto()),
		"sistema":         runtime.GOOS,
	}
}

func (s *Server) getRed(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.estadoRed()})
}

// postRed abre o cierra el agente a la red. Primero cambia el socket y
// solo si eso funciona guarda la configuracion; si guardar falla, vuelve
// a escuchar donde estaba. Asi nunca queda el archivo diciendo una cosa y
// el agente haciendo otra.
func (s *Server) postRed(w http.ResponseWriter, r *http.Request) {
	var pedido struct {
		Activo *bool `json:"activo"`
	}
	if !s.decode(w, r, &pedido) {
		return
	}
	if pedido.Activo == nil {
		s.badRequest(w, `falta "activo": true o false`)
		return
	}
	if s.Reabrir == nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false,
			Error: "este agente no puede cambiar donde escucha sin reiniciarse"})
		return
	}
	s.redCambio.Lock()
	defer s.redCambio.Unlock()

	activo := *pedido.Activo
	hostNuevo := hostSoloEsta
	if activo {
		hostNuevo = hostRed
	}
	hostViejo, _, _ := s.red()

	// Se parte del archivo, como en saveCORS: la configuracion en memoria
	// lleva cosas de la linea de comandos que no hay que guardar.
	cfg, err := config.Load(s.paths.Config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false,
			Error: "no se pudo leer la configuracion: " + err.Error()})
		return
	}
	cfg.Host, cfg.AllowRemote = hostNuevo, activo
	// Abierto a la red sin token, cualquiera de la WiFi podria imprimir y
	// abrir el cajon: se genera uno, como al arrancar.
	if activo && cfg.AuthToken == "" {
		token, err := config.NewToken()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, response{OK: false,
				Error: "no se pudo generar el token: " + err.Error()})
			return
		}
		cfg.AuthToken = token
	}

	if err := s.Reabrir(hostNuevo); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	if err := config.Save(s.paths.Config, cfg); err != nil {
		vuelta := ""
		if err2 := s.Reabrir(hostViejo); err2 != nil {
			vuelta = " Y no se pudo volver atras: " + err2.Error()
		}
		writeJSON(w, http.StatusInternalServerError, response{OK: false,
			Error: "no se pudo guardar la configuracion: " + err.Error() + vuelta})
		return
	}

	s.redMu.Lock()
	s.cfg.Host, s.cfg.AllowRemote, s.cfg.AuthToken = hostNuevo, activo, cfg.AuthToken
	// Una conexion de antes del cambio no prueba que ahora llegue.
	s.conexiones = nil
	s.redMu.Unlock()

	if s.logger != nil {
		s.logger.Info("acceso_red_cambiado", map[string]any{"activo": activo, "host": hostNuevo})
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.estadoRed()})
}

// conectar es la pagina que se abre desde otro aparato para comprobar que
// llega por la red. No pide token (desde la barra de direcciones del movil
// no se pueden mandar cabeceras) y por eso no ensena nada: ni token, ni
// configuracion, ni impresoras.
func (s *Server) conectar(w http.ResponseWriter, r *http.Request) {
	if _, remoto, _ := s.red(); !remoto {
		http.Error(w, "El acceso desde la red esta desactivado.", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if isLocalRequest(r) || mismaMaquina(r) {
		// Desde esta misma PC no se cruza la red ni el cortafuegos: si
		// contara, la prueba saldria bien aunque nadie de fuera llegara.
		// Tambien vale para la IP de red de esta PC, que es la que ensena
		// el panel: el trafico va por dentro de la maquina.
		fmt.Fprint(w, paginaConectar("Esta es la misma PC",
			"Abre esta direccion desde otro aparato de la red (un celular u otra computadora) para comprobar que llega."))
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	s.redMu.Lock()
	s.conexiones = append([]Conexion{{IP: ip, Hora: time.Now()}}, s.conexiones...)
	if len(s.conexiones) > maxConexiones {
		s.conexiones = s.conexiones[:maxConexiones]
	}
	s.redMu.Unlock()
	fmt.Fprint(w, paginaConectar("Conectado a CollaTech Agent",
		"Este aparato llega al agente por la red. Ya puedes cerrar esta pagina."))
}

// mismaMaquina dice si la peticion sale de la misma IP en la que entra,
// es decir, de esta PC aunque use su direccion de red.
func mismaMaquina(r *http.Request) bool {
	local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok {
		return false
	}
	ipLocal, _, err := net.SplitHostPort(local.String())
	if err != nil {
		return false
	}
	ipRemota, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	a, b := net.ParseIP(ipLocal), net.ParseIP(ipRemota)
	return a != nil && b != nil && a.Equal(b)
}

func paginaConectar(titulo, texto string) string {
	return `<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + titulo + `</title>
<style>body{font-family:system-ui,sans-serif;margin:0;min-height:100vh;display:grid;place-items:center;background:#f6f7f9;color:#14171c}
main{max-width:28rem;padding:24px;text-align:center}h1{font-size:1.6rem}p{line-height:1.5}
@media (prefers-color-scheme:dark){body{background:#14171c;color:#eef0f3}}</style>
</head><body><main><h1>` + titulo + `</h1><p>` + texto + `</p></main></body></html>`
}

// qrRed devuelve el QR de la direccion de prueba, para abrirla con la
// camara del movil sin teclearla.
func (s *Server) qrRed(w http.ResponseWriter, r *http.Request) {
	urls, _ := s.estadoRed()["urls_prueba"].([]string)
	if len(urls) == 0 {
		http.Error(w, "esta PC no tiene una IP de red", http.StatusNotFound)
		return
	}
	code, err := qr.Encode(urls[0], qr.M)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(code.PNG())
}
