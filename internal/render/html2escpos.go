package render

import (
	"collatech-agent/internal/escpos"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type HTMLToESCPOS struct {
	builder     *escpos.Builder
	cols        int
	hasContent  bool
	// Table state
	inTable     bool
	tableRows   [][]string
	tableRow    []string
	tableCell   strings.Builder
	tableCols   []int
}

func NewHTMLToESCPOS() *HTMLToESCPOS {
	return &HTMLToESCPOS{
		builder: escpos.New(),
		cols:    32,
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
	h.builder.Feed(1).Cut()
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

func (h *HTMLToESCPOS) processHTML(html string) {
	html = strings.TrimSpace(html)
	if html == "" {
		return
	}

	tokens := tokenizeHTML(html)

	for _, tok := range tokens {
		if tok.text != "" {
			text := decodeHTMLEntities(tok.text)
			text = strings.TrimSpace(text)
			if text != "" {
				if h.inTable {
					if h.tableCell.Len() > 0 {
						h.tableCell.WriteString(" ")
					}
					h.tableCell.WriteString(text)
				} else {
					lines := strings.Split(text, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line != "" {
							h.builder.TextLine(line)
							h.hasContent = true
						}
					}
				}
			}
			continue
		}

		tag := strings.ToLower(tok.tag)

		if tok.isSelf {
			switch tag {
			case "br":
				if h.hasContent {
					h.builder.Line()
				}
			case "hr":
				if h.hasContent {
					h.builder.Line()
				}
				h.builder.TextLine(strings.Repeat("-", h.cols))
				h.hasContent = true
				if h.hasContent {
					h.builder.Line()
				}
			}
			continue
		}

		if tok.isOpen {
			h.handleOpenTag(tag, tok.attrs)
		} else {
			h.handleCloseTag(tag)
		}
	}
}

func (h *HTMLToESCPOS) handleOpenTag(tag, attrs string) {
	switch tag {
	case "br":
		if h.hasContent {
			h.builder.Line()
		}
	case "hr":
		if h.hasContent {
			h.builder.Line()
		}
		h.builder.TextLine(strings.Repeat("-", h.cols))
		h.hasContent = true
		if h.hasContent {
			h.builder.Line()
		}
	case "b", "strong":
		h.builder.Bold(true)
	case "i", "em":
		h.builder.Underline(true)
	case "u":
		h.builder.Underline(true)
	case "h1", "h2", "h3":
		h.builder.AlignCenter().Bold(true).DoubleSize(true)
	case "h4", "h5", "h6":
		h.builder.AlignCenter().Bold(true)
	case "p", "div":
		align := extractAlign(attrs)
		h.applyAlign(align)
	case "center":
		h.builder.AlignCenter()
	case "table":
		h.inTable = true
		h.tableRows = nil
		h.tableRow = nil
		h.tableCell.Reset()
	case "tr":
		h.tableRow = nil
	case "td", "th":
		h.tableCell.Reset()
	}
}

func (h *HTMLToESCPOS) handleCloseTag(tag string) {
	switch tag {
	case "b", "strong":
		h.builder.Bold(false)
	case "i", "em":
		h.builder.Underline(false)
	case "u":
		h.builder.Underline(false)
	case "h1", "h2", "h3":
		h.builder.DoubleSize(false).Bold(false).AlignLeft()
	case "h4", "h5", "h6":
		h.builder.Bold(false).AlignLeft()
	case "p", "div":
		if h.hasContent {
			h.builder.Line()
		}
		h.builder.AlignLeft()
	case "center":
		h.builder.AlignLeft()
	case "td", "th":
		cellText := strings.TrimSpace(h.tableCell.String())
		h.tableRow = append(h.tableRow, cellText)
		h.tableCell.Reset()
	case "tr":
		if len(h.tableRow) > 0 {
			h.tableRows = append(h.tableRows, h.tableRow)
		}
	case "table":
		h.inTable = false
		h.renderTable()
	}
}

func tokenizeHTML(html string) []htmlToken {
	var tokens []htmlToken
	i := 0
	n := len(html)

	for i < n {
		if html[i] == '<' {
			if i+1 < n && html[i+1] == '!' {
				end := strings.Index(html[i:], "-->")
				if end == -1 {
					end = n - i
				}
				i += end + 3
				continue
			}

			end := strings.Index(html[i:], ">")
			if end == -1 {
				text := html[i:]
				if text != "" {
					tokens = append(tokens, htmlToken{text: text})
				}
				break
			}

			tagContent := html[i+1 : i+end]
			i += end + 1

			if len(tagContent) > 0 && tagContent[0] == '/' {
				tagContent = tagContent[1:]
				tag, attrs := parseTag(tagContent)
				tokens = append(tokens, htmlToken{tag: tag, attrs: attrs, isOpen: false})
			} else if len(tagContent) > 0 && tagContent[len(tagContent)-1] == '/' {
				tagContent = tagContent[:len(tagContent)-1]
				tag, attrs := parseTag(tagContent)
				tokens = append(tokens, htmlToken{tag: tag, attrs: attrs, isSelf: true})
			} else {
				tag, attrs := parseTag(tagContent)
				tokens = append(tokens, htmlToken{tag: tag, attrs: attrs, isOpen: true})
			}
		} else {
			end := strings.Index(html[i:], "<")
			if end == -1 {
				text := html[i:]
				if text != "" {
					tokens = append(tokens, htmlToken{text: text})
				}
				break
			}
			text := html[i : i+end]
			if text != "" {
				tokens = append(tokens, htmlToken{text: text})
			}
			i += end
		}
	}

	return tokens
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

func extractAlign(attrs string) string {
	re := regexp.MustCompile(`(?i)text-align\s*:\s*(\w+)`)
	m := re.FindStringSubmatch(attrs)
	if len(m) >= 2 {
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

func stripAllTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "\n")
	reSpace := regexp.MustCompile(`[ \t]+`)
	s = reSpace.ReplaceAllString(s, " ")
	reNL := regexp.MustCompile(`\n\s+`)
	s = reNL.ReplaceAllString(s, "\n")
	reMultiNL := regexp.MustCompile(`\n{3,}`)
	s = reMultiNL.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func (h *HTMLToESCPOS) renderTable() {
	if len(h.tableRows) == 0 {
		return
	}
	// Determinar numero de columnas
	numCols := 0
	for _, row := range h.tableRows {
		if len(row) > numCols {
			numCols = len(row)
		}
	}
	if numCols == 0 {
		return
	}
	// Calcular ancho maximo por columna
	colWidths := make([]int, numCols)
	// Ancho minimo 5 por columna
	for i := range colWidths {
		colWidths[i] = 5
	}
	for _, row := range h.tableRows {
		for i, cell := range row {
			cl := len([]rune(cell))
			if cl > colWidths[i] {
				colWidths[i] = cl
			}
		}
	}
	// Ajustar al ancho total disponible
	totalW := 0
	for _, w := range colWidths {
		totalW += w
	}
	if totalW > h.cols {
		ratio := float64(h.cols) / float64(totalW)
		for i := range colWidths {
			w := int(float64(colWidths[i]) * ratio)
			if w < 3 { w = 3 }
			colWidths[i] = w
		}
		// Rellenar
		adj := h.cols
		for _, w := range colWidths {
			adj -= w
		}
		for adj > 0 {
			for i := range colWidths {
				if adj <= 0 { break }
				colWidths[i]++
				adj--
			}
		}
	}

	border := !h.hasContent // si no hay contenido previo, usar borde
	if border {
		// Borde superior
		var top string
		for i, w := range colWidths {
			top += "┌" + strings.Repeat("─", w)
			if i < numCols-1 { top += "┬" } else { top += "┐" }
		}
		h.builder.TextLine(top)
	}
	for ri, row := range h.tableRows {
		var line string
		for ci := 0; ci < numCols; ci++ {
			txt := ""
			if ci < len(row) {
				txt = row[ci]
			}
			r := []rune(txt)
			w := colWidths[ci]
			if len(r) > w { r = r[:w] }
			txt = string(r) + strings.Repeat(" ", w-len(r))
			if border {
				if ci == 0 { line += "│ " + txt + " │" } else { line = line[:len(line)-1] + txt + " │" }
			} else {
				if ci > 0 { line += "  " }
				line += txt
			}
		}
		h.builder.TextLine(line)
		h.hasContent = true
		if border && ri == 0 && len(h.tableRows) > 1 {
			var mid string
			for i, w := range colWidths {
				mid += "├" + strings.Repeat("─", w)
				if i < numCols-1 { mid += "┼" } else { mid += "┤" }
			}
			h.builder.TextLine(mid)
		}
	}
	if border {
		var bot string
		for i, w := range colWidths {
			bot += "└" + strings.Repeat("─", w)
			if i < numCols-1 { bot += "┴" } else { bot += "┘" }
		}
		h.builder.TextLine(bot)
	}
}

func decodeHTMLEntities(s string) string {
	entities := map[string]string{
		"&amp;":    "&",
		"&lt;":     "<",
		"&gt;":     ">",
		"&quot;":   "\"",
		"&apos;":   "'",
		"&nbsp;":   " ",
		"&#39;":    "'",
		"&eacute;": "é",
		"&Eacute;": "É",
		"&aacute;": "á",
		"&Aacute;": "Á",
		"&oacute;": "ó",
		"&Oacute;": "Ó",
		"&uacute;": "ú",
		"&Uacute;": "Ú",
		"&ntilde;": "ñ",
		"&Ntilde;": "Ñ",
		"&iacute;": "í",
		"&Iacute;": "Í",
		"&uuml;":   "u",
		"&Uuml;":   "U",
		"&auml;":   "a",
		"&Auml;":   "A",
		"&ouml;":   "o",
		"&Ouml;":   "O",
		"&mdash;":  "-",
		"&ndash;":  "-",
		"&rsquo;":  "'",
		"&lsquo;":  "'",
		"&rdquo;":  "\"",
		"&ldquo;":  "\"",
		"&bull;":   "*",
		"&hellip;": "...",
		"&copy;":   "(C)",
		"&reg;":    "(R)",
		"&deg;":    "o",
		"&plusmn;": "+/-",
		"&times;":  "x",
		"&divide;": "/",
	}
	for entity, char := range entities {
		s = strings.ReplaceAll(s, entity, char)
	}
	numRe := regexp.MustCompile(`&#(\d+);`)
	s = numRe.ReplaceAllStringFunc(s, func(m string) string {
		numStr := m[2 : len(m)-1]
		if n, err := strconv.Atoi(numStr); err == nil && n >= 32 {
			return string(rune(n))
		}
		return "?"
	})
	hexRe := regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	s = hexRe.ReplaceAllStringFunc(s, func(m string) string {
		hexStr := m[3 : len(m)-1]
		if n, err := strconv.ParseInt(hexStr, 16, 32); err == nil && n >= 32 {
			return string(rune(n))
		}
		return "?"
	})
	return s
}

func init() {
	_ = unicode.Digit
}
