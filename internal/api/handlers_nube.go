// Configuracion del conector con el backend en la nube, desde el panel.
package api

import (
	"net/http"
	"net/url"
	"strings"

	"collatech-agent/internal/config"
	"collatech-agent/internal/nube"
)

// SetNube conecta el panel con el conector. Lo llama main al arrancar.
func (s *Server) SetNube(c *nube.Conector, cfg config.Cloud) {
	s.nubeMu.Lock()
	s.nube, s.nubeCfg = c, cfg
	s.nubeMu.Unlock()
}

type nubeRespuesta struct {
	URL              string      `json:"url"`
	TokenSet         bool        `json:"token_set"`
	TokenHint        string      `json:"token_hint,omitempty"`
	PollSeconds      int         `json:"poll_seconds,omitempty"`
	HeartbeatSeconds int         `json:"heartbeat_seconds,omitempty"`
	Status           nube.Estado `json:"status"`
}

func (s *Server) nubeActual() nubeRespuesta {
	s.nubeMu.Lock()
	cfg, c := s.nubeCfg, s.nube
	s.nubeMu.Unlock()
	r := nubeRespuesta{
		URL: cfg.URL, TokenSet: cfg.Token != "",
		PollSeconds: cfg.PollSeconds, HeartbeatSeconds: cfg.HeartbeatSeconds,
	}
	// El token no se devuelve entero: basta para reconocer cual es.
	if n := len(cfg.Token); n > 4 {
		r.TokenHint = "..." + cfg.Token[n-4:]
	}
	if c != nil {
		r.Status = c.Estado()
	}
	return r
}

func (s *Server) getCloud(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.nubeActual()})
}

func (s *Server) saveCloud(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL *string `json:"url"`
		// Token sin indicar conserva el que habia, para poder cambiar la
		// URL sin volver a pegarlo. Una cadena vacia lo borra.
		Token            *string `json:"token"`
		PollSeconds      int     `json:"poll_seconds"`
		HeartbeatSeconds int     `json:"heartbeat_seconds"`
	}
	if !s.decode(w, r, &req) {
		return
	}
	s.nubeMu.Lock()
	cfg := s.nubeCfg
	s.nubeMu.Unlock()
	if req.URL != nil {
		cfg.URL = strings.TrimRight(strings.TrimSpace(*req.URL), "/")
	}
	if req.Token != nil {
		cfg.Token = strings.TrimSpace(*req.Token)
	}
	cfg.PollSeconds, cfg.HeartbeatSeconds = req.PollSeconds, req.HeartbeatSeconds

	if cfg.URL != "" {
		u, err := url.Parse(cfg.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			s.badRequest(w, "la URL tiene que ser completa, por ejemplo https://mi-app.com/api/collatech")
			return
		}
		if cfg.Token == "" {
			s.badRequest(w, "falta el token de este local: lo genera tu backend (php artisan collatech:agente)")
			return
		}
	}

	// Se guarda en el archivo de configuracion para que sobreviva a un
	// reinicio. Se relee antes para no pisar lo que haya cambiado en el.
	disco, err := config.Load(s.paths.Config)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: "no se pudo leer la configuracion: " + err.Error()})
		return
	}
	disco.Cloud = cfg
	if err := config.Save(s.paths.Config, disco); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: "no se pudo guardar la configuracion: " + err.Error()})
		return
	}

	s.nubeMu.Lock()
	s.nubeCfg = cfg
	c := s.nube
	s.nubeMu.Unlock()
	if c != nil {
		c.Configurar(cfg)
	}
	s.logger.Info("nube_configurada", map[string]any{"url": cfg.URL, "activa": cfg.Enabled()})
	writeJSON(w, http.StatusOK, response{OK: true, Message: "conexion guardada", Data: s.nubeActual()})
}
