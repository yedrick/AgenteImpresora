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
	"strings"
	"time"

	"collatech-agent/internal/config"
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
	if cfg.Aliases == nil {
		current, err := s.loadSettings()
		if err == nil {
			cfg.Aliases = current.Aliases
		}
	}
	saved, err := s.saveSettingsFile(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Message: "settings saved", Data: saved})
}

func (s *Server) getPrinterAliases(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: cfg.Aliases})
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
	cfg.Aliases = normalizeAliases(aliases)
	saved, err := s.saveSettingsFile(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Message: "printer aliases saved", Data: saved.Aliases})
}

func normalizeAliases(in []settings.Alias) []settings.Alias {
	seen := map[string]bool{}
	out := make([]settings.Alias, 0, len(in))
	for _, alias := range in {
		name := strings.ToLower(strings.TrimSpace(alias.Name))
		printer := strings.TrimSpace(alias.Printer)
		if name == "" || printer == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, settings.Alias{
			Name:        name,
			Printer:     printer,
			Description: strings.TrimSpace(alias.Description),
		})
	}
	return out
}

func (s *Server) resolvePrinters(name string) ([]string, string) {
	requested := strings.TrimSpace(name)
	if requested == "" {
		return nil, ""
	}
	cfg, err := s.loadSettings()
	if err != nil {
		s.logger.Error("printer_alias", map[string]any{"error": err.Error(), "requested_printer": requested})
		return splitPrinterTargets(requested), ""
	}
	key := strings.ToLower(requested)
	for _, alias := range cfg.Aliases {
		if strings.ToLower(strings.TrimSpace(alias.Name)) == key && strings.TrimSpace(alias.Printer) != "" {
			return splitPrinterTargets(alias.Printer), alias.Name
		}
	}
	return splitPrinterTargets(requested), ""
}

func splitPrinterTargets(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		printer := strings.TrimSpace(part)
		key := strings.ToLower(printer)
		if printer == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, printer)
	}
	return out
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
