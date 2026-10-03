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
	"collatech-agent/internal/layout"
	"collatech-agent/internal/queue"
	"collatech-agent/internal/render"
)

// --- Texto ------------------------------------------------------------------

type textRequest struct {
	docRequest
	Text string `json:"text"`
}

func (s *Server) printText(w http.ResponseWriter, r *http.Request) {
	var req textRequest
	if !s.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		s.badRequest(w, "hace falta el texto a imprimir")
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	b := escpos.Begin(res.Doc).Text(strings.Trim(req.Text, "\n\r")).Line()
	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
}

// --- Ticket -----------------------------------------------------------------

type qrSpec struct {
	Data string `json:"data"`
	// Size es el lado de cada punto, 1 a 16. Con 0 se calcula segun el ancho
	// del papel y lo que ocupe el contenido.
	Size int `json:"size"`
	// EC es la correccion de errores: L, M, Q o H.
	EC string `json:"ec"`
}

type barcodeSpec struct {
	Data string `json:"data"`
	// Type: code128 (por defecto), ean13, ean8, upca, upce, code39, code93,
	// itf, codabar o pdf417.
	Type string `json:"type"`
	// Height en puntos, 1 a 255. Width es el grosor de la barra fina, 2 a 6;
	// con 0 se calcula para que quepa en el papel.
	Height int `json:"height"`
	Width  int `json:"width"`
	// HRI es donde va el texto legible: none, above, below o both.
	HRI string `json:"hri"`
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

// ticketLine es un elemento del ticket. El campo type decide cual de los
// demas se usa, de forma que un ticket se describe de arriba abajo en un solo
// array en vez de con campos sueltos repartidos por la peticion.
type ticketLine struct {
	// Type: text (por defecto), table, qr, barcode, image, rule o feed.
	Type string `json:"type"`

	Text    string       `json:"text"`
	Table   *tableDef    `json:"table"`
	QR      *qrSpec      `json:"qr"`
	Barcode *barcodeSpec `json:"barcode"`
	Image   string       `json:"image"`
	// Layout es un bloque maquetado en dos dimensiones. Se compone como
	// imagen, que es la unica forma de poner, por ejemplo, un QR al costado
	// del texto: en nativo la impresora solo sabe avanzar lineas.
	Layout *layout.Layout `json:"layout"`
	// Rule es el caracter con el que se dibuja una linea separadora.
	Rule string `json:"rule"`
	// Feed son las lineas en blanco de un elemento de tipo feed.
	Feed int `json:"feed"`

	Align     string `json:"align"`
	Bold      bool   `json:"bold"`
	Underline bool   `json:"underline"`
	// Invert imprime blanco sobre negro.
	Invert bool `json:"invert"`
	// Size por nombre: normal, small, double, wide, tall.
	Size string `json:"size"`
	// ScaleW y ScaleH son el multiplicador exacto, 1 a 8. Mandan sobre Size.
	ScaleW int `json:"scale_w"`
	ScaleH int `json:"scale_h"`

	Gap int  `json:"gap"`
	Box bool `json:"box"`
	ML  int  `json:"ml"`
	MR  int  `json:"mr"`
}

type ticketRequest struct {
	docRequest
	Title string       `json:"title"`
	Lines []ticketLine `json:"lines"`
	Logo  string       `json:"logo"`
	Scale int          `json:"scale"`

	// Atajos para el caso habitual; equivalen a una linea de tipo qr o
	// barcode al final del ticket.
	QR      string `json:"qr"`
	Barcode string `json:"barcode"`

	Border      bool `json:"border"`
	MarginLeft  int  `json:"margin_left"`
	MarginRight int  `json:"margin_right"`
}

func (s *Server) printTicket(w http.ResponseWriter, r *http.Request) {
	var req ticketRequest
	if !s.decode(w, r, &req) {
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if len(req.QR) > escpos.MaxQRLen {
		s.badRequest(w, fmt.Sprintf("el QR admite %d caracteres como maximo", escpos.MaxQRLen))
		return
	}
	if len(req.Barcode) > escpos.MaxBarcodeLen-2 {
		s.badRequest(w, fmt.Sprintf("el codigo de barras admite %d caracteres como maximo", escpos.MaxBarcodeLen-2))
		return
	}
	resumen := sanitizeTicket(req)
	resumen["cut"] = string(res.Doc.Cut) // el modo efectivo, no el que vino
	s.logger.Info("print_ticket", resumen)

	cols := res.Doc.Columns()
	b := escpos.Begin(res.Doc)

	if req.Border {
		b.Text("┌" + strings.Repeat("─", cols-2) + "┐").Line()
	}
	if req.Logo != "" {
		img, err := decodeImage(req.Logo)
		if err != nil {
			s.badRequest(w, "el logo debe ser PNG, JPEG o GIF en base64")
			return
		}
		b.AlignCenter().ImageFit(img, imageWidth(res.Doc.PaperWidth, pick(req.Scale, res.Scale))).Line()
	}
	if req.Title != "" {
		if req.Border {
			applyLine(b, ticketLine{Text: req.Title, Align: "center", Bold: true, Size: "double"}, cols, true, res)
		} else {
			b.AlignCenter().Bold(true).TextScale(2, 2).TextLine(req.Title).TextScale(1, 1).Bold(false)
		}
	}
	for _, line := range req.Lines {
		if line.ML == 0 && req.MarginLeft > 0 {
			line.ML = req.MarginLeft
		}
		if line.MR == 0 && req.MarginRight > 0 {
			line.MR = req.MarginRight
		}
		if err := applyLine(b, line, cols, req.Border, res); err != nil {
			s.badRequest(w, err.Error())
			return
		}
	}
	if req.Border {
		b.Text("└" + strings.Repeat("─", cols-2) + "┘").Line()
	}
	if req.QR != "" {
		b.AlignCenter().QRWith(req.QR, escpos.QROptions{PaperWidth: res.Doc.PaperWidth}).Line()
	}
	if req.Barcode != "" {
		b.AlignCenter().BarcodeWith(req.Barcode, escpos.BarcodeOptions{
			Type: escpos.BarcodeCode128, PaperWidth: res.Doc.PaperWidth,
		}).Line()
	}

	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
}

// sanitizeTicket arma el evento de log SIN el contenido del ticket: nombres
// de clientes, articulos e importes no deben quedar en claro en un archivo
// que GET /api/logs sirve tal cual. Para diagnosticar basta con la forma.
func sanitizeTicket(r ticketRequest) map[string]any {
	m := map[string]any{
		"printer": r.Printer, "width": r.Width, "scale": r.Scale,
		"drawer": r.Drawer, "border": r.Border,
		"feed_top": r.FeedTop, "feed_bottom": r.FeedBottom,
		"margin_left": r.MarginLeft, "margin_right": r.MarginRight,
		"lines": len(r.Lines),
	}
	if r.UpsideDown != nil {
		m["upside_down"] = *r.UpsideDown
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
	tipos := map[string]int{}
	for _, l := range r.Lines {
		t := strings.ToLower(l.Type)
		if t == "" {
			t = "text"
		}
		tipos[t]++
		if l.Box {
			tipos["box"]++
		}
	}
	if len(tipos) > 0 {
		m["elementos"] = tipos
	}
	return m
}

// --- HTML -------------------------------------------------------------------

type htmlRequest struct {
	docRequest
	HTML string `json:"html"`
}

func (s *Server) printHTML(w http.ResponseWriter, r *http.Request) {
	var req htmlRequest
	if !s.decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.HTML) == "" {
		s.badRequest(w, "hace falta el html a imprimir")
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	s.logger.Info("print_html", map[string]any{
		"printer": req.Printer, "width": res.Doc.PaperWidth, "html_len": len(req.HTML),
	})

	body := render.NewHTMLToESCPOS().
		WithColumns(res.Doc.Columns()).
		WithPaperWidth(res.Doc.PaperWidth).
		WithImageScale(res.Scale).
		Body(req.HTML)

	b := escpos.Begin(res.Doc).RawBytes(body)
	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
}

// --- Plantilla --------------------------------------------------------------

type templateRequest struct {
	docRequest
	Template string         `json:"template"`
	Data     map[string]any `json:"data"`
}

func (s *Server) printTemplate(w http.ResponseWriter, r *http.Request) {
	var req templateRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Template == "" {
		s.badRequest(w, "hace falta el nombre de la plantilla")
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	s.logger.Info("print_template", map[string]any{
		"printer": req.Printer, "template": templateName(req.Template), "width": res.Doc.PaperWidth,
	})
	b := buildNativeTemplate(req, res)
	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
}

// --- Imagen y logo ----------------------------------------------------------

type imageRequest struct {
	docRequest
	Image string `json:"image"`
	Scale int    `json:"scale"`
}

func (s *Server) printImage(w http.ResponseWriter, r *http.Request) {
	var req imageRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Image == "" {
		s.badRequest(w, "hace falta la imagen")
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	img, err := decodeImage(req.Image)
	if err != nil {
		s.badRequest(w, "no se pudo leer la imagen: debe ser PNG, JPEG o GIF en base64")
		return
	}
	s.logger.Info("print_image", map[string]any{"printer": req.Printer, "img_len": len(req.Image)})

	b := escpos.Begin(res.Doc).AlignCenter().
		ImageFit(img, imageWidth(res.Doc.PaperWidth, pick(req.Scale, res.Scale)))
	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
}

type logoRequest struct {
	docRequest
	Scale int `json:"scale"`
}

func (s *Server) printLogo(w http.ResponseWriter, r *http.Request) {
	var req logoRequest
	if !s.decode(w, r, &req) {
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	raster, err := logoRaster(s.paths.Logo, imageWidth(res.Doc.PaperWidth, pick(req.Scale, res.Scale)))
	if err != nil {
		s.logger.Error("print_logo", map[string]any{"error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: "no se pudo leer el logo"})
		return
	}
	b := escpos.Begin(res.Doc).RawBytes(raster)
	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
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
			img, _, decErr := image.Decode(f)
			f.Close()
			if decErr == nil {
				logoImg = img
				return
			}
			// El archivo existe pero no se puede leer: antes se quedaba en
			// error para siempre y todo /api/print/logo devolvia 500 hasta
			// reiniciar. Se cae al logo incrustado.
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
// Convertir la imagen es lo caro y el logo no cambia, asi que se hace una vez
// por ancho.
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

// --- Bloque maquetado -------------------------------------------------------

type layoutRequest struct {
	docRequest
	Layout layout.Layout `json:"layout"`
}

// printLayout compone un bloque en dos dimensiones y lo manda como imagen.
// Es mas lento que el texto nativo, asi que esta pensado para la zona del
// ticket que necesita maquetacion de verdad, no para el ticket entero.
func (s *Server) printLayout(w http.ResponseWriter, r *http.Request) {
	var req layoutRequest
	if !s.decode(w, r, &req) {
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	raster, err := renderLayout(req.Layout, res)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	s.logger.Info("print_layout", map[string]any{
		"printer": req.Printer, "width": res.Doc.PaperWidth, "filas": len(req.Layout.Rows),
	})
	b := escpos.Begin(res.Doc).RawBytes(raster)
	b.End(res.Doc)
	s.enqueue(w, req.Printer, res, b.Bytes())
}

// renderLayout compone el bloque al ancho del papel y lo empaqueta.
func renderLayout(l layout.Layout, res resolved) ([]byte, error) {
	l.Width = res.Doc.PaperWidth
	img, err := layout.Render(l)
	if err != nil {
		return nil, err
	}
	return escpos.New().Image(img).Bytes(), nil
}

// --- ESC/POS crudo ----------------------------------------------------------

type rawRequest struct {
	docRequest
	Data   string `json:"data"`
	Base64 bool   `json:"base64"`
}

func (s *Server) printRaw(w http.ResponseWriter, r *http.Request) {
	var req rawRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Data == "" {
		s.badRequest(w, "hacen falta los datos a enviar")
		return
	}
	res, err := s.resolve(req.docRequest)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	s.logger.Info("print_raw", map[string]any{
		"printer": req.Printer, "data_len": len(req.Data), "base64": req.Base64,
	})

	// Sin base64 el cuerpo llega como texto: se codifica en CP850 igual que
	// el resto del agente. El ASCII y los bytes de control no cambian.
	var payload []byte
	if req.Base64 {
		raw, err := decodeBase64(req.Data)
		if err != nil {
			s.badRequest(w, "los datos no son base64 valido")
			return
		}
		payload = raw
	} else {
		payload = escpos.EncodeCP850(req.Data)
	}
	// Los bytes crudos van tal cual: quien los manda controla la impresora
	// entera, asi que no se le anade arranque ni cierre. El corte si se
	// respeta cuando se pide explicitamente.
	if req.Cut.Set && req.Cut.Mode != escpos.CutNone {
		payload = append(payload, escpos.New().Cut(req.Cut.Mode, res.Doc.FeedBottom).Bytes()...)
	}
	s.enqueue(w, req.Printer, res, payload)
}

// --- Encolado ---------------------------------------------------------------

func (s *Server) badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, response{OK: false, Error: msg})
}

func (s *Server) enqueue(w http.ResponseWriter, requested string, res resolved, payload []byte) {
	if int64(len(payload)) > s.cfg.MaxPrintSize {
		s.logger.Error("print_enqueue", map[string]any{
			"error": "contenido demasiado grande", "printer": requested, "size": len(payload),
		})
		writeJSON(w, http.StatusRequestEntityTooLarge, response{OK: false, Error: "el contenido supera max_print_size"})
		return
	}
	jobs, err := s.queue.EnqueueMany(res.Targets, payload)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrQueueFull) {
			// 503 en vez de dejar la peticion colgada hasta el WriteTimeout.
			status = http.StatusServiceUnavailable
		}
		s.logger.Error("print_enqueue", map[string]any{"error": err.Error(), "printer": requested})
		writeJSON(w, status, response{OK: false, Error: err.Error()})
		return
	}
	for _, job := range jobs {
		event := map[string]any{
			"printer": job.Printer, "requested_printer": requested,
			"size": len(payload), "job_id": job.ID,
		}
		if len(res.Targets) > 1 {
			event["printer_targets"] = res.Targets
		}
		s.logger.Info("print_enqueue", event)
	}
	data := any(jobs[0])
	message := "trabajo encolado"
	if len(jobs) > 1 {
		data = jobs
		message = fmt.Sprintf("trabajo encolado en %d impresoras", len(jobs))
	}
	writeJSON(w, http.StatusAccepted, response{OK: true, Message: message, Data: data})
}

// --- Utilidades -------------------------------------------------------------

func pick(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func decodeBase64(s string) ([]byte, error) {
	if i := strings.Index(s, ","); strings.HasPrefix(s, "data:") && i >= 0 {
		s = s[i+1:]
	}
	return base64.StdEncoding.DecodeString(s)
}

// maxImagePixels acota la imagen ANTES de descomprimirla. Un PNG en blanco
// de 20000x20000 ocupa 400 KB comprimido y 767 MB descomprimido: sin esta
// comprobacion, una peticion que cabe de sobra en max_print_size dejaba al
// agente sin memoria.
const maxImagePixels = 12_000_000 // ~3000x4000, de sobra para un logo

func decodeImage(data string) (image.Image, error) {
	raw, err := decodeBase64(data)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("no se reconoce el formato: debe ser PNG, JPEG o GIF")
	}
	if px := cfg.Width * cfg.Height; px <= 0 || px > maxImagePixels {
		return nil, fmt.Errorf("la imagen mide %dx%d; el maximo son %d megapixeles",
			cfg.Width, cfg.Height, maxImagePixels/1_000_000)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}

// printWidth acota el ancho a los tres valores que el agente sabe componer.
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

// imageWidth calcula el ancho de la imagen en puntos, redondeado a multiplo
// de 8 porque el raster se empaqueta por bytes.
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
