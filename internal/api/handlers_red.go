// Acceso desde la red: el interruptor del panel y su estado.
package api

import (
	"fmt"
	"net/http"
	"runtime"
	"strconv"

	"collatech-agent/internal/config"
)

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
	return map[string]any{
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
	s.redMu.Unlock()

	if s.logger != nil {
		s.logger.Info("acceso_red_cambiado", map[string]any{"activo": activo, "host": hostNuevo})
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.estadoRed()})
}
