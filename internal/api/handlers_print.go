// Manejadores de /api/print/*: validan la peticion, construyen los bytes
// ESC/POS y los encolan.
package api

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"
	"sync"

	"collatech-agent/internal/escpos"
	"collatech-agent/internal/queue"
	"collatech-agent/internal/render"
)

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
		b.Feed(escpos.CutFeedLines).Cut()
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
	// El builder descarta un QR o un codigo de barras fuera de rango para no
	// emitir un comando corrupto; aqui se avisa al cliente en vez de que se
	// pierda en silencio.
	if len(req.Barcode) > escpos.MaxBarcodeLen-2 {
		writeJSON(w, http.StatusBadRequest, response{OK: false,
			Error: fmt.Sprintf("el codigo de barras admite %d caracteres como maximo", escpos.MaxBarcodeLen-2)})
		return
	}
	if len(req.QR) > escpos.MaxQRLen {
		writeJSON(w, http.StatusBadRequest, response{OK: false,
			Error: fmt.Sprintf("el QR admite %d caracteres como maximo", escpos.MaxQRLen)})
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
		if fb < escpos.CutFeedLines {
			// La cuchilla esta varias lineas por encima del cabezal: con 1
			// linea de avance se cortaba la ultima linea del ticket.
			fb = escpos.CutFeedLines
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

// sanitizeTicket arma el evento de log SIN el contenido del ticket. Antes se
// volcaba el texto de cada linea, asi que nombres de clientes, articulos e
// importes quedaban en claro en logs/*.jsonl, que GET /api/logs sirve tal cual.
// Para diagnosticar basta con la forma del ticket, no con lo que dice.
func sanitizeTicket(r ticketRequest) map[string]any {
	m := map[string]any{
		"printer": r.Printer, "width": r.Width, "scale": r.Scale,
		"cut": r.Cut, "drawer": r.Drawer, "border": r.Border,
		"feed_top": r.FeedTop, "feed_bottom": r.FeedBottom,
		"margin_left": r.MarginLeft, "margin_right": r.MarginRight,
		"lines": len(r.Lines),
	}
	if r.Title != "" {
		m["title_len"] = len([]rune(r.Title))
	}
	if r.QR != "" {
		m["qr_len"] = len(r.QR)
	}
	if r.Barcode != "" {
		m["barcode_len"] = len(r.Barcode)
	}
	if r.Logo != "" {
		m["logo_len"] = len(r.Logo)
	}
	var tables, boxes int
	for _, l := range r.Lines {
		if l.Table != nil {
			tables++
		}
		if l.Box {
			boxes++
		}
	}
	if tables > 0 {
		m["tables"] = tables
	}
	if boxes > 0 {
		m["boxes"] = boxes
	}
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
	s.logger.Info("print_template", map[string]any{"printer": req.Printer, "template": req.Template, "width": req.Width, "cut": req.Cut})
	b := buildNativeTemplate(req)
	if req.Cut {
		b.Feed(escpos.CutFeedLines).Cut()
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
		b.Feed(escpos.CutFeedLines).Cut()
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
	raster, err := logoRaster(s.paths.Logo, imageWidth(req.Width, req.Scale))
	if err != nil {
		s.logger.Error("print_logo", map[string]any{"error": err.Error(), "printer": req.Printer})
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: "no se pudo leer el logo"})
		return
	}
	b := escpos.New().Initialize().RawBytes(raster)
	if req.Cut {
		b.Feed(escpos.CutFeedLines).Cut()
	}
	s.enqueue(w, req.Printer, b.Bytes())
}

var (
	logoOnce  sync.Once
	logoImg   image.Image
	logoErr   error
	logoRawMu sync.Mutex
	logoRaw   = map[int][]byte{}
)

// loadLogoImage memoriza el logo descodificado: antes se abria y
// descodificaba el PNG del disco en cada POST /api/print/logo.
func loadLogoImage(path string) (image.Image, error) {
	logoOnce.Do(func() {
		if f, err := os.Open(path); err == nil {
			defer f.Close()
			logoImg, _, logoErr = image.Decode(f)
			return
		}
		data, err := webFS.ReadFile("web/LOGO.png")
		if err != nil {
			logoErr = err
			return
		}
		logoImg, _, logoErr = image.Decode(bytes.NewReader(data))
	})
	return logoImg, logoErr
}

// logoRaster devuelve el logo ya escalado y difuminado para un ancho dado.
// Convertir la imagen es lo caro (difuminado Floyd-Steinberg sobre cientos de
// miles de pixeles) y el logo no cambia, asi que se hace una vez por ancho.
func logoRaster(path string, width int) ([]byte, error) {
	logoRawMu.Lock()
	defer logoRawMu.Unlock()
	if cached, ok := logoRaw[width]; ok {
		return cached, nil
	}
	img, err := loadLogoImage(path)
	if err != nil {
		return nil, err
	}
	raster := escpos.New().AlignCenter().ImageFit(img, width).Line().Bytes()
	logoRaw[width] = raster
	return raster, nil
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
	// Sin base64 el cuerpo llega como texto: se codifica en CP850 igual que
	// el resto del agente. Antes se mandaba UTF-8 crudo y cualquier acento
	// salia como basura. El ASCII y los bytes de control no cambian.
	payload := escpos.EncodeCP850(req.Data)
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
	jobs, err := s.queue.EnqueueMany(resolvedPrinters, payload)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrQueueFull) {
			// 503 en vez de dejar la peticion colgada hasta el WriteTimeout.
			status = http.StatusServiceUnavailable
		}
		s.logger.Error("print_enqueue", map[string]any{"error": err.Error(), "printer": printer})
		writeJSON(w, status, response{OK: false, Error: err.Error()})
		return
	}
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
