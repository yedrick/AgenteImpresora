// Package render convierte HTML sencillo en comandos ESC/POS nativos.
//
// No se rasteriza una imagen de la pagina: el texto sale como texto de la
// impresora, nitido y rapido. Eso marca el limite de lo que se soporta, que
// es el formato que cabe en un ticket.
package render

import (
	"html"
	"regexp"
	"strings"

	"collatech-agent/internal/escpos"
)

// segment es un trozo de texto de la linea en curso con el estilo que tenia
// activo. Acumular segmentos en vez de emitir cada nodo por separado es lo
// que permite que "<p>Total: <b>12</b></p>" salga en una sola linea.
type segment struct {
	text      string
	bold      bool
	underline bool
	invert    bool
	scaleW    int
	scaleH    int
	small     bool
}

// style es el formato activo en un punto del documento.
type style struct {
	bold      int
	underline int
	invert    int
	scaleW    int
	scaleH    int
	small     bool
	align     string
}

type HTMLToESCPOS struct {
	builder    *escpos.Builder
	cols       int
	paperWidth int
	imageScale int
	hasContent bool

	segs  []segment
	st    style
	stack []style

	// Estado de lista
	listDepth  int
	listOrder  []bool
	listNumber []int

	// Estado de tabla
	inTable   bool
	tableRows [][]string
	tableRow  []string
	tableCell strings.Builder
	colWidths []int
	rowHeader []bool
	isHeader  bool

	preDepth int
}

func NewHTMLToESCPOS() *HTMLToESCPOS {
	return &HTMLToESCPOS{
		cols:       32,
		paperWidth: 384,
		imageScale: 80,
		st:         style{scaleW: 1, scaleH: 1, align: "left"},
	}
}

func (h *HTMLToESCPOS) WithColumns(cols int) *HTMLToESCPOS {
	if cols > 0 {
		h.cols = cols
	}
	return h
}

func (h *HTMLToESCPOS) WithPaperWidth(dots int) *HTMLToESCPOS {
	if dots > 0 {
		h.paperWidth = dots
	}
	return h
}

func (h *HTMLToESCPOS) WithImageScale(pct int) *HTMLToESCPOS {
	if pct > 0 {
		h.imageScale = pct
	}
	return h
}

// SetColumns se mantiene por compatibilidad.
func (h *HTMLToESCPOS) SetColumns(cols int) { h.WithColumns(cols) }

// Body devuelve solo el contenido, sin arranque ni corte: de eso se encarga
// el documento que lo envuelve.
func (h *HTMLToESCPOS) Body(markup string) []byte {
	h.builder = escpos.New()
	h.processHTML(markup)
	h.flushLine()
	return h.builder.Bytes()
}

// Render devuelve un documento completo, con arranque y corte.
func (h *HTMLToESCPOS) Render(markup string) []byte {
	opt := escpos.DocOptions{PaperWidth: h.paperWidth, Cut: escpos.CutPartial, FeedBottom: escpos.CutFeedLines}
	b := escpos.Begin(opt).RawBytes(h.Body(markup))
	return b.End(opt).Bytes()
}

// RenderNoCut devuelve un documento completo sin cortar el papel.
func (h *HTMLToESCPOS) RenderNoCut(markup string) []byte {
	opt := escpos.DocOptions{PaperWidth: h.paperWidth, Cut: escpos.CutNone}
	b := escpos.Begin(opt).RawBytes(h.Body(markup))
	return b.End(opt).Bytes()
}

type htmlToken struct {
	tag    string
	attrs  attrs
	text   string
	isOpen bool
	isSelf bool
}

func (h *HTMLToESCPOS) processHTML(markup string) {
	markup = strings.TrimSpace(markup)
	if markup == "" {
		return
	}
	for _, tok := range tokenizeHTML(markup) {
		if tok.text != "" {
			h.appendText(html.UnescapeString(tok.text))
			continue
		}
		tag := strings.ToLower(tok.tag)
		if tok.isOpen || tok.isSelf {
			h.handleOpenTag(tag, tok.attrs, tok.isSelf)
			continue
		}
		h.handleCloseTag(tag)
	}
}

// appendText acumula un nodo de texto en la linea en curso colapsando los
// espacios como hace HTML: los saltos de linea del fuente no son saltos de
// linea del ticket. Dentro de <pre> se respeta tal cual.
func (h *HTMLToESCPOS) appendText(text string) {
	if h.preDepth == 0 {
		text = collapseWS(text)
	}
	if text == "" {
		return
	}
	if h.inTable {
		cell := h.tableCell.String()
		if cell != "" && !strings.HasSuffix(cell, " ") && !strings.HasPrefix(text, " ") {
			h.tableCell.WriteString(" ")
		}
		h.tableCell.WriteString(strings.TrimSpace(text))
		return
	}
	if h.preDepth > 0 {
		for i, part := range strings.Split(text, "\n") {
			if i > 0 {
				h.flushLine()
			}
			h.push(part)
		}
		return
	}
	if len(h.segs) == 0 {
		text = strings.TrimLeft(text, " ")
		if text == "" {
			return
		}
	}
	h.push(text)
}

func (h *HTMLToESCPOS) push(text string) {
	h.segs = append(h.segs, segment{
		text:      text,
		bold:      h.st.bold > 0,
		underline: h.st.underline > 0,
		invert:    h.st.invert > 0,
		scaleW:    max1(h.st.scaleW),
		scaleH:    max1(h.st.scaleH),
		small:     h.st.small,
	})
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// flushLine vuelca la linea acumulada con su alineacion y sus estilos.
func (h *HTMLToESCPOS) flushLine() {
	if len(h.segs) == 0 {
		return
	}
	last := len(h.segs) - 1
	h.segs[last].text = strings.TrimRight(h.segs[last].text, " ")
	if h.segs[last].text == "" {
		h.segs = h.segs[:last]
	}
	if len(h.segs) == 0 {
		return
	}
	h.applyAlign(h.st.align)
	for _, sg := range h.segs {
		h.builder.TextScale(sg.scaleW, sg.scaleH)
		if sg.small {
			h.builder.Font("b")
		} else {
			h.builder.Font("a")
		}
		h.builder.Bold(sg.bold).Underline(sg.underline).Inverted(sg.invert).Text(sg.text)
	}
	h.builder.TextScale(1, 1).Font("a").Bold(false).Underline(false).Inverted(false).Line()
	h.segs = nil
	h.hasContent = true
}

func (h *HTMLToESCPOS) blankLine() {
	if h.hasContent {
		h.builder.Line()
	}
}

func (h *HTMLToESCPOS) pushStyle() { h.stack = append(h.stack, h.st) }

func (h *HTMLToESCPOS) popStyle() {
	if n := len(h.stack); n > 0 {
		h.st = h.stack[n-1]
		h.stack = h.stack[:n-1]
	}
}

// applyInline aplica a la linea en curso lo que diga el atributo style.
func (h *HTMLToESCPOS) applyInline(a attrs) {
	st := a.style()
	if v, ok := st["font-weight"]; ok && (v == "bold" || v == "bolder" || v >= "600") {
		h.st.bold++
	}
	if v, ok := st["text-decoration"]; ok && strings.Contains(v, "underline") {
		h.st.underline++
	}
	if scale, ok := fontScale(st["font-size"]); ok {
		h.st.scaleW, h.st.scaleH = scale, scale
		h.st.small = st["font-size"] == "small" || st["font-size"] == "x-small" || st["font-size"] == "xx-small"
	}
	if al := a.align(); al != "" {
		h.st.align = al
	}
}

func (h *HTMLToESCPOS) handleOpenTag(tag string, a attrs, selfClosing bool) {
	switch tag {
	case "br":
		if len(h.segs) > 0 {
			h.flushLine()
		} else {
			h.blankLine()
		}
	case "hr":
		h.flushLine()
		h.builder.AlignLeft().TextLine(strings.Repeat(ruleChar(a), h.cols))
		h.hasContent = true
	case "b", "strong":
		h.pushStyle()
		h.st.bold++
	case "i", "em", "u", "ins":
		h.pushStyle()
		h.st.underline++
	case "mark":
		h.pushStyle()
		h.st.invert++
	case "small":
		h.pushStyle()
		h.st.small = true
	case "big":
		h.pushStyle()
		h.st.scaleW, h.st.scaleH = 2, 2
	case "span", "font":
		h.pushStyle()
		h.applyInline(a)
	case "h1", "h2", "h3", "h4", "h5", "h6":
		h.flushLine()
		h.pushStyle()
		h.st.bold++
		h.st.align = "center"
		// h1 y h2 a doble tamano; de h3 en adelante solo negrita.
		if tag == "h1" || tag == "h2" {
			h.st.scaleW, h.st.scaleH = 2, 2
		}
		h.applyInline(a)
	case "p", "div", "section", "header", "footer":
		h.flushLine()
		h.pushStyle()
		h.st.align = "left"
		h.applyInline(a)
	case "center":
		h.flushLine()
		h.pushStyle()
		h.st.align = "center"
	case "pre":
		h.flushLine()
		h.pushStyle()
		h.st.small = true
		h.preDepth++
	case "ul", "ol":
		h.flushLine()
		h.listDepth++
		h.listOrder = append(h.listOrder, tag == "ol")
		h.listNumber = append(h.listNumber, 0)
	case "li":
		h.flushLine()
		h.appendText(h.bullet())
	case "table":
		h.flushLine()
		h.inTable = true
		h.tableRows, h.tableRow, h.rowHeader, h.colWidths = nil, nil, nil, nil
		h.tableCell.Reset()
	case "tr":
		h.tableRow = nil
		h.isHeader = false
	case "td", "th":
		h.tableCell.Reset()
		if tag == "th" {
			h.isHeader = true
		}
		// El ancho de columna se toma del primer tr que lo declare.
		if wdt := a.int("width"); wdt > 0 && len(h.colWidths) <= len(h.tableRow) {
			h.colWidths = append(h.colWidths, wdt)
		}
	case "img":
		h.flushLine()
		h.drawImage(a)
	case "qr":
		h.flushLine()
		h.drawQR(a)
	case "barcode":
		h.flushLine()
		h.drawBarcode(a)
	case "feed":
		h.flushLine()
		n := a.int("lines")
		if n <= 0 {
			n = 1
		}
		h.builder.Feed(n)
	}
	// Las etiquetas que se cierran en si mismas no dejan estado abierto.
	if selfClosing {
		switch tag {
		case "b", "strong", "i", "em", "u", "ins", "mark", "small", "big", "span", "font":
			h.popStyle()
		}
	}
}

func (h *HTMLToESCPOS) handleCloseTag(tag string) {
	switch tag {
	case "b", "strong", "i", "em", "u", "ins", "mark", "small", "big", "span", "font":
		h.popStyle()
	case "h1", "h2", "h3", "h4", "h5", "h6":
		h.flushLine()
		h.popStyle()
	case "p", "div", "section", "header", "footer":
		h.flushLine()
		h.blankLine()
		h.popStyle()
	case "center":
		h.flushLine()
		h.popStyle()
	case "pre":
		h.flushLine()
		if h.preDepth > 0 {
			h.preDepth--
		}
		h.popStyle()
	case "ul", "ol":
		h.flushLine()
		if h.listDepth > 0 {
			h.listDepth--
			h.listOrder = h.listOrder[:len(h.listOrder)-1]
			h.listNumber = h.listNumber[:len(h.listNumber)-1]
		}
	case "li":
		h.flushLine()
	case "td", "th":
		h.tableRow = append(h.tableRow, strings.TrimSpace(h.tableCell.String()))
		h.tableCell.Reset()
	case "tr":
		if len(h.tableRow) > 0 {
			h.tableRows = append(h.tableRows, h.tableRow)
			h.rowHeader = append(h.rowHeader, h.isHeader)
		}
		h.tableRow = nil
	case "table":
		h.inTable = false
		h.renderTable()
	}
}

// bullet devuelve la marca de un elemento de lista segun su profundidad.
func (h *HTMLToESCPOS) bullet() string {
	if h.listDepth == 0 {
		return "- "
	}
	i := h.listDepth - 1
	sangria := strings.Repeat("  ", i)
	if h.listOrder[i] {
		h.listNumber[i]++
		return sangria + itoa(h.listNumber[i]) + ". "
	}
	marcas := []string{"• ", "- ", "· "}
	return sangria + marcas[i%len(marcas)]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func ruleChar(a attrs) string {
	if v := a.get("char"); v != "" {
		return string([]rune(v)[0])
	}
	return "-"
}

func (h *HTMLToESCPOS) drawImage(a attrs) {
	src := a.get("src")
	if src == "" {
		return
	}
	img, err := decodeImageData(src)
	if err != nil || img == nil {
		return
	}
	width := h.paperWidth * h.imageScale / 100
	if w := a.int("width"); w > 0 && w < width {
		width = w
	}
	h.applyAlign(firstNonEmpty(a.align(), "center"))
	h.builder.ImageFit(img, (width/8)*8).Line()
	h.hasContent = true
}

func (h *HTMLToESCPOS) drawQR(a attrs) {
	data := firstNonEmpty(a.get("data"), a.get("value"), a.get("src"))
	if data == "" {
		return
	}
	h.applyAlign(firstNonEmpty(a.align(), "center"))
	h.builder.QRWith(data, escpos.QROptions{
		ModuleSize: a.int("size"),
		ECLevel:    a.get("ec"),
		PaperWidth: h.paperWidth,
	}).Line()
	h.hasContent = true
}

func (h *HTMLToESCPOS) drawBarcode(a attrs) {
	data := firstNonEmpty(a.get("data"), a.get("value"))
	if data == "" {
		return
	}
	h.applyAlign(firstNonEmpty(a.align(), "center"))
	h.builder.BarcodeWith(data, escpos.BarcodeOptions{
		Type:        escpos.BarcodeTypeByName(a.get("type")),
		HeightDots:  a.int("height"),
		ModuleWidth: a.int("width"),
		HRI:         a.get("hri"),
		PaperWidth:  h.paperWidth,
	}).Line()
	h.hasContent = true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var wsRe = regexp.MustCompile(`[\s\p{Zs}]+`)

func collapseWS(s string) string { return wsRe.ReplaceAllString(s, " ") }

func (h *HTMLToESCPOS) applyAlign(align string) {
	switch align {
	case "center":
		h.builder.AlignCenter()
	case "right":
		h.builder.AlignRight()
	default:
		h.builder.AlignLeft()
	}
}
