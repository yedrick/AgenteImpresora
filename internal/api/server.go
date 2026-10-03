// Package api expone la API REST del agente y el panel web embebido.
//
// El archivo reune el montaje del servidor: estado compartido, tabla de
// rutas y utilidades de peticion y respuesta. Los manejadores viven en
// handlers_*.go.
package api

import (
	"embed"
	"encoding/json"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
	"collatech-agent/internal/settings"
)

//go:embed web/*
var webFS embed.FS

type Server struct {
	cfg      config.Config
	paths    config.Paths
	printers *printers.Manager
	queue    *queue.Manager
	logger   *logs.Logger

	// Los ajustes se leian del disco en CADA impresion, para resolver los
	// alias. Ahora se cachean y solo se releen si cambia el archivo.
	settingsMu   sync.Mutex
	settingsVal  settings.Settings
	settingsStat time.Time
	settingsSize int64
	settingsOK   bool
}

type response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func NewServer(cfg config.Config, paths config.Paths, pm *printers.Manager, qm *queue.Manager, logger *logs.Logger) *Server {
	return &Server{cfg: cfg, paths: paths, printers: pm, queue: qm, logger: logger}
}

// loadSettings devuelve los ajustes, releyendo el archivo solo si cambio su
// tamano o su fecha de modificacion.
func (s *Server) loadSettings() (settings.Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if info, err := os.Stat(s.paths.Settings); err == nil && s.settingsOK {
		if info.ModTime().Equal(s.settingsStat) && info.Size() == s.settingsSize {
			return s.settingsVal, nil
		}
	}
	cfg, err := settings.Load(s.paths.Settings)
	if err != nil {
		return cfg, err
	}
	s.settingsVal, s.settingsOK = cfg, true
	if info, statErr := os.Stat(s.paths.Settings); statErr == nil {
		s.settingsStat, s.settingsSize = info.ModTime(), info.Size()
	}
	return cfg, nil
}

// saveSettingsFile escribe los ajustes y refresca la cache.
func (s *Server) saveSettingsFile(cfg settings.Settings) (settings.Settings, error) {
	saved, err := settings.Save(s.paths.Settings, cfg)
	if err != nil {
		return saved, err
	}
	s.settingsMu.Lock()
	s.settingsVal, s.settingsOK = saved, true
	if info, statErr := os.Stat(s.paths.Settings); statErr == nil {
		s.settingsStat, s.settingsSize = info.ModTime(), info.Size()
	}
	s.settingsMu.Unlock()
	return saved, nil
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.panel)
	mux.HandleFunc("GET /panel", s.panel)
	mux.HandleFunc("GET /designer", s.designer)
	mux.HandleFunc("GET /diagnostico", s.diagnostico)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/network", s.network)
	mux.HandleFunc("GET /api/diagnostico", s.diagnosticReport)
	mux.HandleFunc("GET /api/logs", s.readLogs)
	mux.HandleFunc("GET /api/token", s.readToken)
	mux.HandleFunc("GET /api/support-bundle", s.supportBundle)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /api/printers-config", s.getPrinters)
	mux.HandleFunc("POST /api/printers-config", s.savePrinters)
	mux.HandleFunc("GET /api/printer-aliases", s.getPrinterAliases)
	mux.HandleFunc("POST /api/printer-aliases", s.savePrinterAliases)
	mux.HandleFunc("GET /api/templates", s.listTemplates)
	mux.HandleFunc("GET /api/printers", s.listPrinters)
	mux.HandleFunc("GET /api/models", s.listModels)
	mux.HandleFunc("POST /api/print/text", s.printText)
	mux.HandleFunc("POST /api/print/ticket", s.printTicket)
	mux.HandleFunc("POST /api/print/logo", s.printLogo)
	mux.HandleFunc("POST /api/print/template", s.printTemplate)
	mux.HandleFunc("POST /api/print/html", s.printHTML)
	mux.HandleFunc("POST /api/print/image", s.printImage)
	mux.HandleFunc("POST /api/preview", s.preview)
	mux.HandleFunc("POST /api/print/layout", s.printLayout)
	mux.HandleFunc("POST /api/print/raw", s.printRaw)
	// El orden importa: CORS tiene que responder el preflight OPTIONS antes
	// que el guardia, porque un preflight no puede llevar cabecera de
	// autorizacion.
	var h http.Handler = mux
	h = s.guard(h)
	h = s.rateLimit(h)
	if !s.cfg.AllowRemote {
		h = s.localhostOnly(h)
	}
	h = s.cors(h)
	h = s.recoverPanics(h)
	return h
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxPrintSize)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if err == io.EOF {
			writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "request body is required"})
			return false
		}
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "invalid json: " + err.Error()})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
