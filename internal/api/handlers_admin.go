// Consultas y configuracion: estado, red, impresoras, plantillas, ajustes,
// alias y registro.
package api

import (
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strconv"
	"time"

	"collatech-agent/internal/config"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/settings"
)

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: map[string]any{
		"service": "CollaTech Agent",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"jobs":    s.queue.List(),
	}})
}

func (s *Server) network(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	port := s.cfg.Port
	if port == 0 {
		port = config.DefaultPort
	}
	urls := []string{}
	if host != "" {
		urls = append(urls, fmt.Sprintf("http://%s:%d/panel", host, port))
	}
	for _, ip := range localIPv4s() {
		urls = append(urls, fmt.Sprintf("http://%s:%d/panel", ip, port))
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: map[string]any{
		"hostname":     host,
		"host":         s.cfg.Host,
		"port":         port,
		"allow_remote": s.cfg.AllowRemote,
		"urls":         urls,
	}})
}

func (s *Server) readLogs(w http.ResponseWriter, r *http.Request) {
	limit := 80
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	lines, err := tailLogLines(s.paths.Logs, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: lines})
}

// readToken devuelve el token de acceso a la red. Es admin, asi que solo
// responde a peticiones desde la propia PC: sirve para que el operador lo
// copie del panel y lo configure en las demas estaciones.
func (s *Server) readToken(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: map[string]any{
		"required": s.cfg.AllowRemote && s.cfg.AuthToken != "",
		"token":    s.cfg.AuthToken,
	}})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: cfg})
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var cfg settings.Settings
	if !s.decode(w, r, &cfg) {
		return
	}
	// Si la peticion no trae impresoras, se conservan las que ya hubiera: el
	// panel manda solo las preferencias generales.
	if cfg.Printers == nil {
		if current, err := s.loadSettings(); err == nil {
			cfg.Printers = current.Printers
		}
	}
	saved, err := s.saveSettingsFile(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Message: "settings saved", Data: saved})
}

// nativeTemplates son las plantillas que buildNativeTemplate sabe construir.
// Son la unica fuente de verdad: antes /api/templates listaba los .html de
// disco, que nunca se renderizaban, asi que el panel ofrecia plantillas que
// al imprimirse salian como una factura generica.
var nativeTemplates = []string{"factura", "recibo", "comanda", "texto", "qr", "imagen"}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: nativeTemplates})
}

func (s *Server) listPrinters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.printers.List()})
}

// getPrinters devuelve las impresoras dadas de alta, con sus ajustes.
func (s *Server) getPrinters(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: cfg.Printers})
}

// savePrinters reemplaza la lista de impresoras dadas de alta.
func (s *Server) savePrinters(w http.ResponseWriter, r *http.Request) {
	var list []settings.Printer
	if !s.decode(w, r, &list) {
		return
	}
	cfg, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	cfg.Printers = list
	saved, err := s.saveSettingsFile(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Message: "impresoras guardadas", Data: saved.Printers})
}

// getPrinterAliases y savePrinterAliases se mantienen por compatibilidad con
// los integradores que ya usan /api/printer-aliases; por dentro trabajan
// sobre la misma lista de impresoras.
func (s *Server) getPrinterAliases(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	out := make([]settings.Alias, 0, len(cfg.Printers))
	for _, p := range cfg.Printers {
		out = append(out, settings.Alias{Name: p.Name, Printer: p.Target, Description: p.Description})
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: out})
}

func (s *Server) savePrinterAliases(w http.ResponseWriter, r *http.Request) {
	var aliases []settings.Alias
	if !s.decode(w, r, &aliases) {
		return
	}
	cfg, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	// Se conservan los ajustes propios de cada impresora que ya existiera.
	list := make([]settings.Printer, 0, len(aliases))
	for _, a := range aliases {
		p := settings.Printer{Name: a.Name, Target: a.Printer, Description: a.Description}
		if old, ok := cfg.Find(a.Name); ok {
			old.Target, old.Description = a.Printer, a.Description
			p = old
		}
		list = append(list, p)
	}
	cfg.Printers = list
	saved, err := s.saveSettingsFile(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	out := make([]settings.Alias, 0, len(saved.Printers))
	for _, p := range saved.Printers {
		out = append(out, settings.Alias{Name: p.Name, Printer: p.Target, Description: p.Description})
	}
	writeJSON(w, http.StatusOK, response{OK: true, Message: "impresoras guardadas", Data: out})
}

// listModels devuelve el catalogo de modelos conocidos, con sus capacidades
// y, para el unico caso que lo necesita, el enlace oficial del fabricante.
// El agente no descarga ni instala nada: solo informa.
func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: printers.Catalog})
}
