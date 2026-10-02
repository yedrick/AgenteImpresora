package api

import (
	"bytes"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"collatech-agent/internal/config"
	"collatech-agent/internal/escpos"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
	"collatech-agent/internal/render"
	"collatech-agent/internal/settings"
)

//go:embed web/*
var webFS embed.FS

type Server struct {
	cfg      config.Config
	printers *printers.Manager
	queue    *queue.Manager
	renderer *render.Renderer
	logger   *logs.Logger
}

type response struct {
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

func NewServer(cfg config.Config, pm *printers.Manager, qm *queue.Manager, renderer *render.Renderer, logger *logs.Logger) *Server {
	return &Server{cfg: cfg, printers: pm, queue: qm, renderer: renderer, logger: logger}
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
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /api/printer-aliases", s.getPrinterAliases)
	mux.HandleFunc("POST /api/printer-aliases", s.savePrinterAliases)
	mux.HandleFunc("GET /api/templates", s.listTemplates)
	mux.HandleFunc("GET /api/printers", s.listPrinters)
	mux.HandleFunc("POST /api/print/text", s.printText)
	mux.HandleFunc("POST /api/print/ticket", s.printTicket)
	mux.HandleFunc("POST /api/print/logo", s.printLogo)
	mux.HandleFunc("POST /api/print/template", s.printTemplate)
	mux.HandleFunc("POST /api/print/html", s.printHTML)
	mux.HandleFunc("POST /api/print/image", s.printImage)
	mux.HandleFunc("POST /api/print/raw", s.printRaw)
	if s.cfg.AllowRemote {
		return s.cors(mux)
	}
	return s.localhostOnly(s.cors(mux))
}

func (s *Server) panel(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/index.html")
}

func (s *Server) designer(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/designer.html")
}

func (s *Server) diagnostico(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/diagnostico.html")
}

func (s *Server) serveHTML(w http.ResponseWriter, path string) {
	b, err := webFS.ReadFile(path)
	if err != nil {
		b, err = os.ReadFile(path) // fallback a disco
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{OK: false, Error: path + " not found"})
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Message: "healthy"})
}

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
		port = 18743
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

func (s *Server) diagnosticReport(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	port := s.cfg.Port
	if port == 0 {
		port = 18743
	}
	scheme := "http"
	if s.cfg.TLS.Enabled {
		scheme = "https"
	}
	ips := localIPv4s()
	localURL := fmt.Sprintf("%s://127.0.0.1:%d/health", scheme, port)
	lanChecks := []map[string]any{}
	panelURLs := []string{fmt.Sprintf("%s://127.0.0.1:%d/panel", scheme, port)}
	if host != "" {
		panelURLs = append(panelURLs, fmt.Sprintf("%s://%s:%d/panel", scheme, host, port))
	}
	for _, ip := range ips {
		healthURL := fmt.Sprintf("%s://%s:%d/health", scheme, ip, port)
		panelURLs = append(panelURLs, fmt.Sprintf("%s://%s:%d/panel", scheme, ip, port))
		lanChecks = append(lanChecks, map[string]any{
			"ip":     ip,
			"url":    healthURL,
			"result": probeURL(healthURL),
		})
	}

	configRaw := readTextFile("configs/config.json", 64*1024)
	report := map[string]any{
		"generated_at": time.Now().Format(time.RFC3339),
		"service":      "CollaTech Agent",
		"runtime": map[string]any{
			"os":          runtime.GOOS,
			"arch":        runtime.GOARCH,
			"pid":         os.Getpid(),
			"executable":  exe,
			"working_dir": cwd,
		},
		"server": map[string]any{
			"scheme":       scheme,
			"host":         s.cfg.Host,
			"port":         port,
			"listen_addr":  fmt.Sprintf("%s:%d", s.cfg.Host, port),
			"allow_remote": s.cfg.AllowRemote,
			"tls_enabled":  s.cfg.TLS.Enabled,
			"panel_urls":   panelURLs,
		},
		"network": map[string]any{
			"hostname":        host,
			"local_ipv4":      ips,
			"localhost_check": probeURL(localURL),
			"lan_checks":      lanChecks,
		},
		"firewall": firewallDiagnostics(port, exe),
		"config": map[string]any{
			"path":   filepath.Join(cwd, "configs", "config.json"),
			"active": s.cfg,
			"raw":    configRaw,
		},
		"advice": diagnosticAdvice(s.cfg, ips),
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: report})
}

func localIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) readLogs(w http.ResponseWriter, r *http.Request) {
	limit := 80
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	lines, err := tailLogLines("logs", limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: lines})
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Load("storage/settings.json")
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
		current, err := settings.Load("storage/settings.json")
		if err == nil {
			cfg.Aliases = current.Aliases
		}
	}
	saved, err := settings.Save("storage/settings.json", cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Message: "settings saved", Data: saved})
}

func (s *Server) getPrinterAliases(w http.ResponseWriter, r *http.Request) {
	cfg, err := settings.Load("storage/settings.json")
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
	cfg, err := settings.Load("storage/settings.json")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	cfg.Aliases = normalizeAliases(aliases)
	saved, err := settings.Save("storage/settings.json", cfg)
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
	cfg, err := settings.Load("storage/settings.json")
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

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir("templates")
	if err != nil {
		writeJSON(w, http.StatusOK, response{OK: true, Data: []string{
			"comanda.html",
			"factura.html",
			"imagen.html",
			"qr.html",
			"recibo.html",
			"texto.html",
			"ticket.html",
		}})
		return
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".html") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	writeJSON(w, http.StatusOK, response{OK: true, Data: names})
}

func (s *Server) listPrinters(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.printers.List()})
}

type textRequest struct {
	Printer string `json:"printer"`
	Text    string `json:"text"`
	Cut     bool   `json:"cut"`
}

func (s *Server) printText(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" || strings.TrimSpace(req.Text) == "" {
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer and text are required"})
		return
	}
	b := escpos.New().Initialize().Text(strings.Trim(req.Text, "\n\r")).Line()
	if req.Cut {
		b.Feed(1).Cut()
	}
	s.enqueue(w, req.Printer, b.Bytes())
}

type ticketRequest struct {
	Printer     string       `json:"printer"`
	Title       string       `json:"title"`
	Lines       []ticketLine `json:"lines"`
	QR          string       `json:"qr"`
	Barcode     string       `json:"barcode"`
	Logo        string       `json:"logo"`
	Width       int          `json:"width"`
	Scale       int          `json:"scale"`
	Cut         bool         `json:"cut"`
	Drawer      bool         `json:"drawer"`
	FeedTop     int          `json:"feed_top"`
	FeedBottom  int          `json:"feed_bottom"`
	Border      bool         `json:"border"`
	MarginLeft  int          `json:"margin_left"`
	MarginRight int          `json:"margin_right"`
}

type tableColumn struct {
	Text  string `json:"text"`
	Width int    `json:"width"`
	Align string `json:"align"`
}

type tableDef struct {
	Border  bool          `json:"border"`
	Header  bool          `json:"header"`
	Columns []tableColumn `json:"columns"`
	Rows    [][]string    `json:"rows"`
}

type ticketLine struct {
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	Table     *tableDef `json:"table"`
	Align     string    `json:"align"`
	Bold      bool      `json:"bold"`
	Underline bool      `json:"underline"`
	Size      string    `json:"size"`
	Gap       int       `json:"gap"`
	Box       bool      `json:"box"`
	ML        int       `json:"ml"`
	MR        int       `json:"mr"`
}

func (s *Server) printTicket(w http.ResponseWriter, r *http.Request) {
	var req ticketRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" {
		s.logger.Error("print_ticket", map[string]any{"error": "printer is required", "body": sanitizeTicket(req)})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer is required"})
		return
	}
	s.logger.Info("print_ticket", sanitizeTicket(req))
	cols := ticketCols(req.Width)
	b := escpos.New().Initialize()
	if req.FeedTop > 0 {
		b.Feed(req.FeedTop)
	}
	if req.Drawer {
		b.DrawerKick()
	}
	if req.Border {
		b.Text("┌" + strings.Repeat("─", cols-2) + "┐").Line()
	}
	if req.Logo != "" {
		img, err := decodeImage(req.Logo)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "logo must be base64 PNG/JPEG/GIF"})
			return
		}
		b.AlignCenter().ImageFit(img, imageWidth(req.Width, req.Scale)).Line()
	}
	if req.Title != "" {
		if req.Border {
			applyLine(b, ticketLine{Text: req.Title, Align: "center", Bold: true, Size: "double"}, cols, true)
		} else {
			b.AlignCenter().Bold(true).DoubleSize(true).TextLine(req.Title).DoubleSize(false).Bold(false)
		}
	}
	for _, line := range req.Lines {
		if line.ML == 0 && req.MarginLeft > 0 {
			line.ML = req.MarginLeft
		}
		if line.MR == 0 && req.MarginRight > 0 {
			line.MR = req.MarginRight
		}
		applyLine(b, line, cols, req.Border)
	}
	if req.Border {
		b.Text("└" + strings.Repeat("─", cols-2) + "┘").Line()
	}
	if req.QR != "" {
		b.AlignCenter().QR(req.QR).Line()
	}
	if req.Barcode != "" {
		b.AlignCenter().Barcode(req.Barcode).Line()
	}
	if req.Cut {
		fb := req.FeedBottom
		if fb < 1 {
			fb = 1
		}
		b.Feed(fb).Cut()
	}
	s.enqueue(w, req.Printer, b.Bytes())
}

type htmlRequest struct {
	Printer string `json:"printer"`
	HTML    string `json:"html"`
	Width   int    `json:"width"`
	Cut     bool   `json:"cut"`
}

func sanitizeTicket(r ticketRequest) map[string]any {
	m := map[string]any{
		"printer": r.Printer, "width": r.Width, "scale": r.Scale,
		"cut": r.Cut, "drawer": r.Drawer, "border": r.Border,
		"feed_top": r.FeedTop, "feed_bottom": r.FeedBottom,
		"margin_left": r.MarginLeft, "margin_right": r.MarginRight,
	}
	if r.Title != "" {
		m["title"] = r.Title
	}
	if r.QR != "" {
		m["qr"] = r.QR
	}
	if r.Barcode != "" {
		m["barcode"] = r.Barcode
	}
	if r.Logo != "" {
		m["logo"] = "base64:" + fmt.Sprint(len(r.Logo))
	}
	lines := make([]map[string]any, len(r.Lines))
	for i, l := range r.Lines {
		lm := map[string]any{"text": l.Text, "type": l.Type, "align": l.Align, "bold": l.Bold}
		if l.Table != nil {
			lm["table"] = l.Table
		}
		if l.Gap > 0 {
			lm["gap"] = l.Gap
		}
		if l.Size != "" {
			lm["size"] = l.Size
		}
		lines[i] = lm
	}
	m["lines"] = lines
	return m
}

func (s *Server) printHTML(w http.ResponseWriter, r *http.Request) {
	var req htmlRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" || strings.TrimSpace(req.HTML) == "" {
		s.logger.Error("print_html", map[string]any{"error": "printer and html are required", "printer": req.Printer})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer and html are required"})
		return
	}
	s.logger.Info("print_html", map[string]any{"printer": req.Printer, "width": req.Width, "cut": req.Cut, "html_len": len(req.HTML)})
	html2esc := render.NewHTMLToESCPOS()
	if req.Width > 0 {
		cols := 32
		if req.Width >= 576 {
			cols = 48
		} else if req.Width >= 512 {
			cols = 42
		}
		html2esc.SetColumns(cols)
	}
	var data []byte
	if req.Cut {
		data = html2esc.Render(req.HTML)
	} else {
		data = html2esc.RenderNoCut(req.HTML)
	}
	s.enqueue(w, req.Printer, data)
}

type templateRequest struct {
	Printer  string         `json:"printer"`
	Template string         `json:"template"`
	Data     map[string]any `json:"data"`
	Width    int            `json:"width"`
	Cut      bool           `json:"cut"`
}

func (s *Server) printTemplate(w http.ResponseWriter, r *http.Request) {
	var req templateRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" || req.Template == "" {
		s.logger.Error("print_template", map[string]any{"error": "printer and template are required", "printer": req.Printer, "template": req.Template})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer and template are required"})
		return
	}
	if strings.Contains(req.Template, "..") || strings.ContainsAny(req.Template, `/\`) {
		s.logger.Error("print_template", map[string]any{"error": "invalid template name", "template": req.Template})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "invalid template name"})
		return
	}
	s.logger.Info("print_template", map[string]any{"printer": req.Printer, "template": req.Template, "width": req.Width, "cut": req.Cut})
	b := buildNativeTemplate(req)
	if req.Cut {
		b.Feed(1).Cut()
	}
	s.enqueue(w, req.Printer, b.Bytes())
}

type imageRequest struct {
	Printer string `json:"printer"`
	Image   string `json:"image"`
	Width   int    `json:"width"`
	Scale   int    `json:"scale"`
	Cut     bool   `json:"cut"`
}

func (s *Server) printImage(w http.ResponseWriter, r *http.Request) {
	var req imageRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" || req.Image == "" {
		s.logger.Error("print_image", map[string]any{"error": "printer and image are required", "printer": req.Printer})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer and image are required"})
		return
	}
	s.logger.Info("print_image", map[string]any{"printer": req.Printer, "width": req.Width, "cut": req.Cut, "img_size": len(req.Image)})
	img, err := decodeImage(req.Image)
	if err != nil {
		s.logger.Error("print_image", map[string]any{"error": "could not decode image", "printer": req.Printer})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "could not decode image"})
		return
	}
	b := escpos.New().Initialize().AlignCenter().ImageFit(img, imageWidth(req.Width, req.Scale))
	if req.Cut {
		b.Feed(1).Cut()
	}
	s.enqueue(w, req.Printer, b.Bytes())
}

type logoRequest struct {
	Printer string `json:"printer"`
	Width   int    `json:"width"`
	Scale   int    `json:"scale"`
	Cut     bool   `json:"cut"`
}

func (s *Server) printLogo(w http.ResponseWriter, r *http.Request) {
	var req logoRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" {
		s.logger.Error("print_logo", map[string]any{"error": "printer is required"})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer is required"})
		return
	}
	s.logger.Info("print_logo", map[string]any{"printer": req.Printer, "width": req.Width, "cut": req.Cut})
	img, err := loadLogoImage()
	if err != nil {
		s.logger.Error("print_logo", map[string]any{"error": "could not decode LOGO.png", "printer": req.Printer})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "could not decode LOGO.png"})
		return
	}
	b := escpos.New().Initialize().AlignCenter().ImageFit(img, imageWidth(req.Width, req.Scale)).Line()
	if req.Cut {
		b.Feed(1).Cut()
	}
	s.enqueue(w, req.Printer, b.Bytes())
}

func loadLogoImage() (image.Image, error) {
	f, err := os.Open("LOGO.png")
	if err == nil {
		defer f.Close()
		img, _, decodeErr := image.Decode(f)
		return img, decodeErr
	}
	data, err := webFS.ReadFile("web/LOGO.png")
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

type rawRequest struct {
	Printer string `json:"printer"`
	Data    string `json:"data"`
	Base64  bool   `json:"base64"`
}

func (s *Server) printRaw(w http.ResponseWriter, r *http.Request) {
	var req rawRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Printer == "" || req.Data == "" {
		s.logger.Error("print_raw", map[string]any{"error": "printer and data are required", "printer": req.Printer})
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer and data are required"})
		return
	}
	s.logger.Info("print_raw", map[string]any{"printer": req.Printer, "data_len": len(req.Data), "base64": req.Base64})
	payload := []byte(req.Data)
	if req.Base64 {
		raw, err := decodeBase64(req.Data)
		if err != nil {
			s.logger.Error("print_raw", map[string]any{"error": "invalid base64 data", "printer": req.Printer})
			writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "invalid base64 data"})
			return
		}
		payload = raw
	}
	s.enqueue(w, req.Printer, payload)
}

func (s *Server) enqueue(w http.ResponseWriter, printer string, payload []byte) {
	if int64(len(payload)) > s.cfg.MaxPrintSize {
		s.logger.Error("print_enqueue", map[string]any{"error": "payload too large", "printer": printer, "size": len(payload)})
		writeJSON(w, http.StatusRequestEntityTooLarge, response{OK: false, Error: "print payload is too large"})
		return
	}
	resolvedPrinters, alias := s.resolvePrinters(printer)
	if len(resolvedPrinters) == 0 {
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "printer is required"})
		return
	}
	jobs := s.queue.EnqueueMany(resolvedPrinters, payload)
	for _, job := range jobs {
		event := map[string]any{"printer": job.Printer, "requested_printer": printer, "size": len(payload), "job_id": job.ID}
		if alias != "" {
			event["printer_alias"] = alias
		}
		if len(resolvedPrinters) > 1 {
			event["printer_targets"] = resolvedPrinters
		}
		s.logger.Info("print_enqueue", event)
	}
	data := any(jobs[0])
	message := "print job queued"
	if len(jobs) > 1 {
		data = jobs
		message = "print jobs queued"
	}
	writeJSON(w, http.StatusAccepted, response{OK: true, Message: message, Data: data})
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

func (s *Server) localhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.RemoteAddr
		if !(strings.HasPrefix(host, "127.0.0.1:") || strings.HasPrefix(host, "[::1]:") || strings.HasPrefix(host, "localhost:")) {
			writeJSON(w, http.StatusForbidden, response{OK: false, Error: "local connections only"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.originAllowed(origin) {
			if hasWildcard(s.cfg.AllowedCORS) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) originAllowed(origin string) bool {
	for _, allowed := range s.cfg.AllowedCORS {
		if allowed == "*" || strings.HasPrefix(origin, allowed) {
			return true
		}
	}
	return false
}

func hasWildcard(allowed []string) bool {
	for _, item := range allowed {
		if item == "*" {
			return true
		}
	}
	return false
}

func drawTable(b *escpos.Builder, tbl *tableDef, cols int, ticketBorder bool, ml, mr int) {
	if tbl == nil || len(tbl.Columns) == 0 {
		return
	}
	if ml < 0 {
		ml = 0
	}
	if mr < 0 {
		mr = 0
	}
	inner := cols - ml - mr
	if inner < 8 {
		inner = 8
	}

	if ticketBorder {
		inner -= 2
	}
	if inner < 6 {
		inner = 6
	}
	colCount := len(tbl.Columns)
	sep := tbl.Border

	// Calcular anchos totales
	colWidths := make([]int, colCount)
	totalWidth := 0
	for i, c := range tbl.Columns {
		w := c.Width
		if w < 1 {
			w = 5
		}
		// Ajustar si excede el ancho disponible
		if totalWidth+w > inner {
			w = inner - totalWidth
			if w < 3 {
				w = 3
			}
		}
		colWidths[i] = w
		totalWidth += w
		if sep && i < colCount-1 {
			totalWidth++ // para │ separador
		}
	}
	// Si no alcanza, escalar proporcionalmente
	if totalWidth > inner {
		ratio := float64(inner) / float64(totalWidth)
		newTotal := 0
		for i := range colWidths {
			w := int(float64(colWidths[i]) * ratio)
			if w < 3 {
				w = 3
			}
			colWidths[i] = w
			newTotal += w
			if sep && i < colCount-1 {
				newTotal++
			}
		}
		// Ajustar ultima columna para llenar exactamente
		if newTotal < inner {
			colWidths[colCount-1] += inner - newTotal
		}
	}

	b.AlignLeft().FontSize("normal")
	padLine := func(s string) string {
		if ml > 0 || mr > 0 {
			s = strings.Repeat(" ", ml) + s + strings.Repeat(" ", mr)
		}
		return s
	}

	buildRow := func(cells []string, isHeader bool) {
		var line string
		for i, cell := range cells {
			txt := cell
			r := []rune(txt)
			w := colWidths[i]
			if len(r) > w {
				r = r[:w]
			}
			switch strings.ToLower(tbl.Columns[i].Align) {
			case "center":
				pad := w - len(r)
				left := pad / 2
				right := pad - left
				txt = strings.Repeat(" ", left) + string(r) + strings.Repeat(" ", right)
			case "right":
				txt = strings.Repeat(" ", w-len(r)) + string(r)
			default:
				txt = string(r) + strings.Repeat(" ", w-len(r))
			}
			if sep {
				line += "│" + txt
			} else {
				if i > 0 {
					line += " "
				}
				line += txt
			}
		}
		if sep {
			line += "│"
		}
		if ticketBorder && !sep {
			// Rellenar hacia la derecha para que la linea mida inner
			rline := []rune(line)
			if len(rline) < inner {
				line += strings.Repeat(" ", inner-len(rline))
			}
		}
		if isHeader && tbl.Header && sep {
			b.Bold(true).TextLine(padLine(line)).Bold(false)
		} else {
			b.TextLine(padLine(line))
		}
	}

	// Borde superior
	if sep {
		var top string
		for i, w := range colWidths {
			if i == 0 {
				top += "┌"
			} else {
				top += "┬"
			}
			top += strings.Repeat("─", w)
		}
		top += "┐"
		if ticketBorder {
			// Centrar dentro del border
			r := []rune(top)
			pad := (inner - len(r)) / 2
			if pad > 0 {
				top = strings.Repeat(" ", pad) + top
			}
		}
		b.TextLine(padLine(top))
	}

	// Header
	if len(tbl.Rows) > 0 && tbl.Header {
		buildRow(tbl.Rows[0], true)
		// Separador header
		if sep {
			var mid string
			for i, w := range colWidths {
				if i == 0 {
					mid += "├"
				} else {
					mid += "┼"
				}
				mid += strings.Repeat("─", w)
			}
			mid += "┤"
			if ticketBorder {
				r := []rune(mid)
				pad := (inner - len(r)) / 2
				if pad > 0 {
					mid = strings.Repeat(" ", pad) + mid
				}
			}
			b.TextLine(padLine(mid))
		}
	}

	// Filas de datos
	startRow := 0
	if tbl.Header {
		startRow = 1
	}
	for i := startRow; i < len(tbl.Rows); i++ {
		buildRow(tbl.Rows[i], false)
	}

	// Borde inferior
	if sep {
		var bot string
		for i, w := range colWidths {
			if i == 0 {
				bot += "└"
			} else {
				bot += "┴"
			}
			bot += strings.Repeat("─", w)
		}
		bot += "┘"
		if ticketBorder {
			r := []rune(bot)
			pad := (inner - len(r)) / 2
			if pad > 0 {
				bot = strings.Repeat(" ", pad) + bot
			}
		}
		b.TextLine(padLine(bot))
	}
	b.FontSize("normal").Bold(false)
}

func applyLine(b *escpos.Builder, line ticketLine, cols int, border bool) {
	if strings.ToLower(line.Type) == "table" {
		if line.Table != nil {
			ml := line.ML
			mr := line.MR
			if ml < 0 {
				ml = 0
			}
			if mr < 0 {
				mr = 0
			}
			drawTable(b, line.Table, cols, border, ml, mr)
		}
		return
	}

	text := line.Text
	r := []rune(text)

	if line.Box {
		drawBoxLine(b, line, r, cols, border)
		return
	}

	ml := line.ML
	mr := line.MR
	if ml < 0 {
		ml = 0
	}
	if mr < 0 {
		mr = 0
	}

	if border {
		b.AlignLeft()
		inner := cols - 2 - ml - mr
		if inner < 1 {
			inner = 1
		}
		if len(r) > inner {
			r = r[:inner]
		}
		switch strings.ToLower(line.Align) {
		case "center":
			pad := inner - len(r)
			left := pad / 2
			right := pad - left
			text = strings.Repeat(" ", ml) + strings.Repeat(" ", left) + string(r) + strings.Repeat(" ", right) + strings.Repeat(" ", mr)
		case "right":
			text = strings.Repeat(" ", ml) + strings.Repeat(" ", inner-len(r)) + string(r) + strings.Repeat(" ", mr)
		default:
			text = strings.Repeat(" ", ml) + string(r) + strings.Repeat(" ", inner-len(r)) + strings.Repeat(" ", mr)
		}
		text = "│" + text + "│"
	} else {
		text = strings.Repeat(" ", ml) + text + strings.Repeat(" ", mr)
		switch strings.ToLower(line.Align) {
		case "center":
			b.AlignCenter()
		case "right":
			b.AlignRight()
		default:
			b.AlignLeft()
		}
	}
	b.FontSize(line.Size).Bold(line.Bold).Underline(line.Underline).TextLine(text)
	for i := 0; i < line.Gap; i++ {
		b.Line()
	}
	b.FontSize("normal").Bold(false).Underline(false)
}

func drawBoxLine(b *escpos.Builder, line ticketLine, r []rune, cols int, border bool) {
	inner := cols
	if border {
		inner = cols - 2
	}
	maxText := inner - 4
	if maxText < 1 {
		maxText = 1
	}
	if len(r) > maxText {
		r = r[:maxText]
	}
	ml := line.ML
	mr := line.MR
	if ml < 0 {
		ml = 0
	}
	if mr < 0 {
		mr = 0
	}
	if ml+mr > 0 {
		r = append([]rune(strings.Repeat(" ", ml)), r...)
		r = append(r, []rune(strings.Repeat(" ", mr))...)
	}
	wall := len(r) + 4
	if wall+2 > inner {
		wall = inner - 2
		if wall < 4 {
			wall = 4
		}
	}
	topLine := "┌" + strings.Repeat("─", wall) + "┐"
	ctLine := "│  " + string(r) + strings.Repeat(" ", wall-len(r)-2) + " │"
	botLine := "└" + strings.Repeat("─", wall) + "┘"

	b.FontSize("normal").Bold(line.Bold)

	if border {
		pad := 0
		switch strings.ToLower(line.Align) {
		case "center":
			pad = (inner - (wall + 2)) / 2
		case "right":
			pad = inner - (wall + 2)
		}
		if pad < 0 {
			pad = 0
		}
		rp := inner - (wall + 2) - pad
		if rp < 0 {
			rp = 0
		}
		b.AlignLeft()
		b.TextLine("│" + strings.Repeat(" ", pad) + topLine + strings.Repeat(" ", rp) + "│")
		b.TextLine("│" + strings.Repeat(" ", pad) + ctLine + strings.Repeat(" ", rp) + "│")
		b.TextLine("│" + strings.Repeat(" ", pad) + botLine + strings.Repeat(" ", rp) + "│")
	} else {
		switch strings.ToLower(line.Align) {
		case "center":
			b.AlignCenter()
		case "right":
			b.AlignRight()
		default:
			b.AlignLeft()
		}
		b.TextLine(topLine)
		b.TextLine(ctLine)
		b.TextLine(botLine)
	}

	b.FontSize("normal").Bold(false)
	for i := 0; i < line.Gap; i++ {
		b.Line()
	}
}

func ticketCols(width int) int {
	switch {
	case width >= 576:
		return 48
	case width >= 512:
		return 42
	default:
		return 32
	}
}

func buildNativeTemplate(req templateRequest) *escpos.Builder {
	data := req.Data
	template := strings.ToLower(req.Template)
	empresa := cleanText(dataString(data, "empresa", "COLLATECH"))
	cliente := cleanText(dataString(data, "cliente", "Cliente Demo"))
	total := cleanText(dataString(data, "total", "0.00"))
	mensaje := cleanText(dataString(data, "mensaje", "Gracias por su compra"))
	qr := dataString(data, "qr", "")
	barcode := dataString(data, "barcode", "")
	items := dataItems(data)

	b := escpos.New().Initialize()
	b.AlignCenter().Bold(true).DoubleSize(true).TextLine(empresa).DoubleSize(false).Bold(false)

	switch {
	case strings.Contains(template, "recibo"):
		b.AlignCenter().TextLine("RECIBO")
		b.AlignLeft().TextLine("Cliente: " + cliente)
		b.TextLine("--------------------------------")
		b.Bold(true).TextLine(padBoth("TOTAL PAGADO", total, 32)).Bold(false)
	case strings.Contains(template, "comanda"):
		b.AlignCenter().TextLine("COMANDA")
		b.AlignLeft().TextLine("Mesa/Cliente: " + cliente)
		b.TextLine("--------------------------------")
		for _, item := range items {
			b.TextLine("- " + cleanText(item.Name))
		}
	case strings.Contains(template, "texto"):
		b.AlignLeft().TextLine(mensaje)
		b.TextLine(time.Now().Format("2006-01-02 15:04:05"))
	case strings.Contains(template, "qr"):
		b.AlignCenter().TextLine("QR DE PRUEBA")
		if qr == "" {
			qr = "https://kollatek.com"
		}
		b.QR(qr).Line()
	case strings.Contains(template, "imagen"):
		b.AlignCenter().TextLine("[ LOGO / IMAGEN ]")
		b.TextLine(mensaje)
	default:
		b.AlignCenter().TextLine("FACTURA")
		b.AlignLeft().TextLine("Cliente: " + cliente)
		b.TextLine(time.Now().Format("2006-01-02 15:04:05"))
		// Tabla de items
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{"1", cleanText(item.Name), cleanText(item.Price)})
		}
		drawTable(b, &tableDef{
			Border: false,
			Header: true,
			Columns: []tableColumn{
				{Text: "CANT", Width: 4, Align: "center"},
				{Text: "PRODUCTO", Width: 16, Align: "left"},
				{Text: "PRECIO", Width: 12, Align: "right"},
			},
			Rows: rows,
		}, 32, false, 0, 0)
		b.TextLine("--------------------------------")
		b.Bold(true).TextLine(padBoth("TOTAL", total, 32)).Bold(false)
	}

	if qr != "" && !strings.Contains(template, "qr") {
		b.AlignCenter().Line().QR(qr).Line()
	}
	if barcode != "" {
		b.AlignCenter().Barcode(barcode).Line()
	}
	b.AlignCenter().TextLine(mensaje)
	return b
}

type nativeItem struct {
	Name  string
	Price string
}

func dataString(data map[string]any, key, fallback string) string {
	if data == nil {
		return fallback
	}
	v, ok := data[key]
	if !ok || v == nil {
		return fallback
	}
	switch val := v.(type) {
	case string:
		if strings.TrimSpace(val) == "" {
			return fallback
		}
		return val
	default:
		return strings.TrimSpace(strings.Trim(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(toJSON(val)), "\n", " "), "\"", ""), "{}"))
	}
}

func dataItems(data map[string]any) []nativeItem {
	if data == nil {
		return []nativeItem{{Name: "Producto Demo", Price: "Bs 10.00"}}
	}
	raw, ok := data["items"].([]any)
	if !ok || len(raw) == 0 {
		return []nativeItem{{Name: "Producto Demo", Price: "Bs 10.00"}}
	}
	items := make([]nativeItem, 0, len(raw))
	for _, row := range raw {
		itemMap, ok := row.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, nativeItem{
			Name:  dataString(itemMap, "nombre", "Item"),
			Price: dataString(itemMap, "precio", ""),
		})
	}
	if len(items) == 0 {
		return []nativeItem{{Name: "Producto Demo", Price: "Bs 10.00"}}
	}
	return items
}

func padBoth(left, right string, width int) string {
	left = cleanText(left)
	right = cleanText(right)
	if width <= 0 {
		width = 32
	}
	maxLeft := width - len(right) - 1
	if maxLeft < 1 {
		return left + " " + right
	}
	if len(left) > maxLeft {
		left = left[:maxLeft]
	}
	spaces := width - len(left) - len(right)
	if spaces < 1 {
		spaces = 1
	}
	return left + strings.Repeat(" ", spaces) + right
}

func cleanText(s string) string {
	replacer := strings.NewReplacer(
		"–", "-", "—", "-", "“", "\"", "”", "\"", "’", "'",
	)
	s = replacer.Replace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var out strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || (r >= 32) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeBase64(s string) ([]byte, error) {
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i >= 0 {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}

func decodeImage(data string) (image.Image, error) {
	raw, err := decodeBase64(data)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

func printWidth(width int) int {
	switch {
	case width >= 576:
		return 576
	case width >= 512:
		return 512
	default:
		return 384
	}
}

func imageWidth(width, scale int) int {
	base := printWidth(width)
	if scale <= 0 {
		scale = 80
	}
	if scale < 35 {
		scale = 35
	}
	if scale > 100 {
		scale = 100
	}
	out := base * scale / 100
	out = (out / 8) * 8
	if out < 8 {
		out = 8
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, payload response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func probeURL(url string) map[string]any {
	start := time.Now()
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	elapsed := time.Since(start).Milliseconds()
	out := map[string]any{
		"url": url,
		"ok":  false,
		"ms":  elapsed,
	}
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer resp.Body.Close()
	out["status"] = resp.StatusCode
	out["ok"] = resp.StatusCode >= 200 && resp.StatusCode < 300
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	out["body"] = strings.TrimSpace(string(body))
	return out
}

func firewallDiagnostics(port int, exe string) map[string]any {
	return map[string]any{
		"os":           runtime.GOOS,
		"port_rule":    runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=GOServer"+strconv.Itoa(port)),
		"legacy_rule":  runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=CollaTech Agent 18743"),
		"program_rule": runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=CollaTech Agent App"),
		"listen":       runDiagnosticCommand(4*time.Second, "netstat", "-ano", "-p", "tcp"),
		"exe":          exe,
	}
}

func runDiagnosticCommand(timeout time.Duration, name string, args ...string) map[string]any {
	if runtime.GOOS != "windows" && (strings.EqualFold(name, "netsh") || strings.EqualFold(name, "netstat")) {
		return map[string]any{"ok": false, "error": "command only available on Windows"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if len(text) > 20000 {
		text = text[:20000] + "\n...[truncated]"
	}
	result := map[string]any{
		"command": strings.TrimSpace(name + " " + strings.Join(args, " ")),
		"ok":      err == nil,
		"output":  text,
	}
	if ctx.Err() == context.DeadlineExceeded {
		result["error"] = "timeout"
		return result
	}
	if err != nil {
		result["error"] = err.Error()
	}
	return result
}

func readTextFile(path string, max int64) map[string]any {
	out := map[string]any{"path": path, "ok": false}
	f, err := os.Open(path)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max))
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	out["ok"] = true
	out["text"] = string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}))
	return out
}

func diagnosticAdvice(cfg config.Config, ips []string) []string {
	var advice []string
	if cfg.Host == "127.0.0.1" || strings.EqualFold(cfg.Host, "localhost") {
		advice = append(advice, "El host esta en modo local. Para acceso desde celular usa host 0.0.0.0.")
	}
	if !cfg.AllowRemote {
		advice = append(advice, "allow_remote esta desactivado. Las peticiones externas seran bloqueadas por el agente.")
	}
	if cfg.TLS.Enabled {
		advice = append(advice, "TLS/HTTPS esta activado. Debes probar con https:// y un certificado valido o desactivar TLS para red local.")
	}
	if len(ips) == 0 {
		advice = append(advice, "No se detecto una IPv4 LAN activa. Revisa WiFi/Ethernet.")
	}
	if len(advice) == 0 {
		advice = append(advice, "La configuracion del agente parece apta para LAN. Si el celular no entra, revisa firewall externo, antivirus o aislamiento WiFi del router.")
	}
	return advice
}

func tailLogLines(dir string, limit int) ([]json.RawMessage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []json.RawMessage{}, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var lines []json.RawMessage
	for i := len(names) - 1; i >= 0 && len(lines) < limit; i-- {
		b, err := os.ReadFile(filepath.Join(dir, names[i]))
		if err != nil {
			return nil, err
		}
		fileLines := strings.Split(strings.TrimSpace(string(b)), "\n")
		for j := len(fileLines) - 1; j >= 0 && len(lines) < limit; j-- {
			line := strings.TrimSpace(fileLines[j])
			if line != "" {
				lines = append(lines, json.RawMessage(line))
			}
		}
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}
