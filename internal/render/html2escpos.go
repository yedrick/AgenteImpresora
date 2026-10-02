package render

import (
	"html"
	"regexp"
	"strings"

	"collatech-agent/internal/escpos"
)

// segment es un trozo de texto de la linea en curso junto con el estilo que
// tenia activo cuando se leyo. Acumular segmentos en vez de emitir cada nodo
// de texto por separado es lo que permite que "<p>Total: <b>12</b></p>" salga
// en una sola linea.
type segment struct {
	text      string
	bold      bool
	underline bool
	size      string
}

type HTMLToESCPOS struct {
	builder    *escpos.Builder
	cols       int
	hasContent bool

	// Linea en curso
	segs      []segment
	boldDepth int
	undDepth  int
	size      string
	align     string

	// Estado de tabla
	inTable   bool
	tableRows [][]string
	tableRow  []string
	tableCell strings.Builder
}

func NewHTMLToESCPOS() *HTMLToESCPOS {
	return &HTMLToESCPOS{
		builder: escpos.New(),
		cols:    32,
		size:    "normal",
		align:   "left",
	}
}

func (h *HTMLToESCPOS) SetColumns(cols int) {
	if cols > 0 {
		h.cols = cols
	}
}

func (h *HTMLToESCPOS) Render(html string) []byte {
	h.builder.Initialize()
	h.processHTML(html)
	h.builder.Feed(escpos.CutFeedLines).Cut()
	return h.builder.Bytes()
}

func (h *HTMLToESCPOS) RenderNoCut(html string) []byte {
	h.builder.Initialize()
	h.processHTML(html)
	return h.builder.Bytes()
}

type htmlToken struct {
	tag    string
	attrs  string
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
		if tok.isSelf {
			h.handleOpenTag(tag, tok.attrs)
			continue
		}
		if tok.isOpen {
			h.handleOpenTag(tag, tok.attrs)
		} else {
			h.handleCloseTag(tag)
		}
	}
	h.flushLine()
}

// appendText acumula un nodo de texto en la linea en curso colapsando los
// espacios como hace HTML: los saltos de linea del fuente no son saltos de
// linea del ticket.
func (h *HTMLToESCPOS) appendText(text string) {
	text = collapseWS(text)
	if text == "" {
		return
	}
	if h.inTable {
		if h.tableCell.Len() > 0 && !strings.HasSuffix(h.tableCell.String(), " ") {
			h.tableCell.WriteString(" ")
		}
		h.tableCell.WriteString(strings.TrimSpace(text))
		return
	}
	if len(h.segs) == 0 {
		text = strings.TrimLeft(text, " ")
		if text == "" {
			return
		}
	}
	h.segs = append(h.segs, segment{
		text:      text,
		bold:      h.boldDepth > 0,
		underline: h.undDepth > 0,
		size:      h.size,
	})
}

// flushLine vuelca la linea acumulada con su alineacion y sus estilos.
func (h *HTMLToESCPOS) flushLine() {
	if len(h.segs) == 0 {
		return
	}
	// Quitar el espacio sobrante del final de la linea.
	last := len(h.segs) - 1
	h.segs[last].text = strings.TrimRight(h.segs[last].text, " ")
	if h.segs[last].text == "" {
		h.segs = h.segs[:last]
	}
	if len(h.segs) == 0 {
		return
	}

	h.applyAlign(h.align)
	for _, sg := range h.segs {
		size := sg.size
		if size == "" {
			size = "normal"
		}
		h.builder.FontSize(size).Bold(sg.bold).Underline(sg.underline).Text(sg.text)
	}
	h.builder.FontSize("normal").Bold(false).Underline(false).Line()
	h.segs = nil
	h.hasContent = true
}

func (h *HTMLToESCPOS) blankLine() {
	if h.hasContent {
		h.builder.Line()
	}
}

func (h *HTMLToESCPOS) handleOpenTag(tag, attrs string) {
	switch tag {
	case "br":
		if len(h.segs) > 0 {
			h.flushLine()
		} else {
			h.blankLine()
		}
	case "hr":
		h.flushLine()
		h.builder.AlignLeft().TextLine(strings.Repeat("-", h.cols))
		h.hasContent = true
	case "b", "strong":
		h.boldDepth++
	case "i", "em", "u":
		h.undDepth++
	case "h1", "h2", "h3":
		h.flushLine()
		h.align = "center"
		h.boldDepth++
		h.size = "double"
	case "h4", "h5", "h6":
		h.flushLine()
		h.align = "center"
		h.boldDepth++
	case "p", "div":
		h.flushLine()
		if a := extractAlign(attrs); a != "" {
			h.align = a
		} else {
			h.align = "left"
		}
	case "center":
		h.flushLine()
		h.align = "center"
	case "li":
		h.flushLine()
		h.appendText("- ")
	case "tr":
		h.tableRow = nil
	case "td", "th":
		h.tableCell.Reset()
	case "table":
		h.flushLine()
		h.inTable = true
		h.tableRows = nil
		h.tableRow = nil
		h.tableCell.Reset()
	}
}

func (h *HTMLToESCPOS) handleCloseTag(tag string) {
	switch tag {
	case "b", "strong":
		h.boldDepth = dec(h.boldDepth)
	case "i", "em", "u":
		h.undDepth = dec(h.undDepth)
	case "h1", "h2", "h3":
		h.flushLine()
		h.size = "normal"
		h.boldDepth = dec(h.boldDepth)
		h.align = "left"
	case "h4", "h5", "h6":
		h.flushLine()
		h.boldDepth = dec(h.boldDepth)
		h.align = "left"
	case "p", "div":
		h.flushLine()
		h.blankLine()
		h.align = "left"
	case "center":
		h.flushLine()
		h.align = "left"
	case "li":
		h.flushLine()
	case "td", "th":
		h.tableRow = append(h.tableRow, strings.TrimSpace(h.tableCell.String()))
		h.tableCell.Reset()
	case "tr":
		if len(h.tableRow) > 0 {
			h.tableRows = append(h.tableRows, h.tableRow)
		}
		h.tableRow = nil
	case "table":
		h.inTable = false
		h.renderTable()
	}
}

func dec(n int) int {
	if n > 0 {
		return n - 1
	}
	return 0
}

var wsRe = regexp.MustCompile(`[\s\p{Zs}]+`)

func collapseWS(s string) string {
	return wsRe.ReplaceAllString(s, " ")
}

// rawTextTags son elementos cuyo contenido NO es texto para imprimir. Sin esto,
// el CSS de cualquier HTML real salia impreso en el ticket.
var rawTextTags = map[string]bool{"style": true, "script": true, "title": true, "head": true}

func tokenizeHTML(markup string) []htmlToken {
	var tokens []htmlToken
	i := 0
	n := len(markup)

	for i < n {
		if markup[i] != '<' {
			end := strings.Index(markup[i:], "<")
			if end == -1 {
				tokens = append(tokens, htmlToken{text: markup[i:]})
				break
			}
			if end > 0 {
				tokens = append(tokens, htmlToken{text: markup[i : i+end]})
			}
			i += end
			continue
		}

		if i+1 < n && markup[i+1] == '!' {
			// Comentario: consumir hasta "-->".
			if strings.HasPrefix(markup[i:], "<!--") {
				end := strings.Index(markup[i+4:], "-->")
				if end == -1 {
					break
				}
				i += 4 + end + 3
				continue
			}
			// Declaracion tipo <!DOCTYPE html>: consumir solo hasta el '>'.
			// Tratarla como comentario descartaba el documento entero.
			end := strings.Index(markup[i:], ">")
			if end == -1 {
				break
			}
			i += end + 1
			continue
		}

		end := strings.Index(markup[i:], ">")
		if end == -1 {
			tokens = append(tokens, htmlToken{text: markup[i:]})
			break
		}
		tagContent := markup[i+1 : i+end]
		i += end + 1

		switch {
		case strings.HasPrefix(tagContent, "/"):
			tag, attrs := parseTag(tagContent[1:])
			tokens = append(tokens, htmlToken{tag: tag, attrs: attrs, isOpen: false})
		case strings.HasSuffix(tagContent, "/"):
			tag, attrs := parseTag(tagContent[:len(tagContent)-1])
			tokens = append(tokens, htmlToken{tag: tag, attrs: attrs, isSelf: true})
		default:
			tag, attrs := parseTag(tagContent)
			tokens = append(tokens, htmlToken{tag: tag, attrs: attrs, isOpen: true})
			if rawTextTags[tag] {
				if skip := indexCloseTag(markup[i:], tag); skip >= 0 {
					i += skip
				} else {
					i = n
				}
			}
		}
	}

	return tokens
}

// indexCloseTag devuelve el desplazamiento del "</tag" mas cercano, sin
// distinguir mayusculas.
func indexCloseTag(s, tag string) int {
	needle := "</" + tag
	lower := strings.ToLower(s)
	return strings.Index(lower, needle)
}

func parseTag(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	parts := strings.Fields(s)
	tag := strings.ToLower(parts[0])
	attrs := ""
	if len(parts) > 1 {
		attrs = strings.Join(parts[1:], " ")
	}
	return tag, attrs
}

var alignRe = regexp.MustCompile(`(?i)text-align\s*:\s*(\w+)`)

func extractAlign(attrs string) string {
	if m := alignRe.FindStringSubmatch(attrs); len(m) >= 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

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

func (h *HTMLToESCPOS) renderTable() {
	rows := h.tableRows
	h.tableRows = nil
	if len(rows) == 0 {
		return
	}
	numCols := 0
	for _, row := range rows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	if numCols == 0 {
		return
	}

	// El marco gasta "|" por cada columna mas uno al cierre, y un espacio de
	// relleno a cada lado de la celda.
	overhead := numCols*3 + 1
	border := overhead+numCols*3 <= h.cols
	avail := h.cols
	if border {
		avail = h.cols - overhead
	} else {
		avail = h.cols - (numCols - 1)
	}
	if avail < numCols {
		avail = numCols
	}

	colWidths := make([]int, numCols)
	for i := range colWidths {
		colWidths[i] = 1
	}
	for _, row := range rows {
		for i, cell := range row {
			if w := len([]rune(cell)); w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}
	total := 0
	for _, w := range colWidths {
		total += w
	}
	if total > avail {
		scale := float64(avail) / float64(total)
		total = 0
		for i := range colWidths {
			w := int(float64(colWidths[i]) * scale)
			if w < 1 {
				w = 1
			}
			colWidths[i] = w
			total += w
		}
	}
	// Repartir lo que sobre en la ultima columna, para que el marco cuadre.
	if total < avail {
		colWidths[numCols-1] += avail - total
	}

	h.builder.AlignLeft()
	if border {
		h.builder.TextLine(tableRule(colWidths, "┌", "┬", "┐"))
	}
	for ri, row := range rows {
		cells := make([]string, numCols)
		for i := 0; i < numCols; i++ {
			txt := ""
			if i < len(row) {
				txt = row[i]
			}
			cells[i] = fitCell(txt, colWidths[i])
		}
		if border {
			h.builder.TextLine("│ " + strings.Join(cells, " │ ") + " │")
		} else {
			h.builder.TextLine(strings.Join(cells, " "))
		}
		h.hasContent = true
		if border && ri == 0 && len(rows) > 1 {
			h.builder.TextLine(tableRule(colWidths, "├", "┼", "┤"))
		}
	}
	if border {
		h.builder.TextLine(tableRule(colWidths, "└", "┴", "┘"))
	}
}

// tableRule construye una linea de marco. Se arma con strings.Join sobre runas
// completas: la version anterior recortaba un byte de un caracter de caja de 3
// bytes y producia UTF-8 invalido.
func tableRule(colWidths []int, left, mid, right string) string {
	parts := make([]string, len(colWidths))
	for i, w := range colWidths {
		parts[i] = strings.Repeat("─", w+2)
	}
	return left + strings.Join(parts, mid) + right
}

func fitCell(text string, width int) string {
	r := []rune(text)
	if len(r) > width {
		r = r[:width]
	}
	return string(r) + strings.Repeat(" ", width-len(r))
}
