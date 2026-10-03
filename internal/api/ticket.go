// Composicion de tickets: tablas, lineas con formato, recuadros y las
// plantillas integradas.
package api

import (
	"encoding/json"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"collatech-agent/internal/escpos"
)

func drawTable(b *escpos.Builder, tbl *tableDef, cols int, ticketBorder bool, ml, mr int) {
	if tbl == nil || len(tbl.Columns) == 0 {
		return
	}
	ml, mr = clampCols(ml), clampCols(mr)
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
		// Se recorren las columnas declaradas, no las celdas: una fila con mas
		// celdas que columnas desbordaba colWidths, y una con menos dejaba la
		// linea corta.
		for i := 0; i < colCount; i++ {
			txt := ""
			if i < len(cells) {
				txt = cells[i]
			}
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

// applyLine dibuja un elemento del ticket. El campo type decide que se
// pinta; sin type se imprime texto, que es el caso de siempre.
func applyLine(b *escpos.Builder, line ticketLine, cols int, border bool, res resolved) error {
	switch strings.ToLower(strings.TrimSpace(line.Type)) {
	case "table":
		if line.Table != nil {
			drawTable(b, line.Table, cols, border, clampMin0(line.ML), clampMin0(line.MR))
		}
		return nil

	case "qr":
		// Antes esto devolvia nil: la linea se esfumaba, el trabajo decia
		// "Completed" y la factura salia sin el QR de pago sin que nadie se
		// enterara. Mas vale un error claro.
		if line.QR == nil {
			return fmt.Errorf(`una linea de tipo "qr" necesita el contenido: {"type":"qr","qr":"https://..."}`)
		}
		if line.QR.Data == "" {
			return fmt.Errorf(`el QR esta vacio: ponle contenido o quita la linea`)
		}
		if len(line.QR.Data) > escpos.MaxQRLen {
			return fmt.Errorf("el QR admite %d caracteres como maximo", escpos.MaxQRLen)
		}
		alignOf(b, line.Align, "center")
		b.QRWith(line.QR.Data, escpos.QROptions{
			ModuleSize: line.QR.Size,
			ECLevel:    line.QR.EC,
			PaperWidth: res.Doc.PaperWidth,
		}).Line()
		return feedGap(b, line)

	case "barcode":
		if line.Barcode == nil {
			return fmt.Errorf(`una linea de tipo "barcode" necesita el contenido: {"type":"barcode","barcode":"123456789"}`)
		}
		if line.Barcode.Data == "" {
			return fmt.Errorf(`el codigo de barras esta vacio: ponle contenido o quita la linea`)
		}
		if len(line.Barcode.Data) > escpos.MaxBarcodeLen-2 {
			return fmt.Errorf("el codigo de barras admite %d caracteres como maximo", escpos.MaxBarcodeLen-2)
		}
		alignOf(b, line.Align, "center")
		b.BarcodeWith(line.Barcode.Data, escpos.BarcodeOptions{
			Type:        escpos.BarcodeTypeByName(line.Barcode.Type),
			HeightDots:  line.Barcode.Height,
			ModuleWidth: line.Barcode.Width,
			HRI:         line.Barcode.HRI,
			PaperWidth:  res.Doc.PaperWidth,
		}).Line()
		return feedGap(b, line)

	case "layout":
		if line.Layout == nil {
			return fmt.Errorf(`una linea de tipo "layout" necesita el bloque: {"type":"layout","layout":{"rows":[...]}}`)
		}
		raster, err := renderLayout(*line.Layout, res)
		if err != nil {
			return fmt.Errorf("bloque maquetado: %w", err)
		}
		b.AlignLeft().RawBytes(raster)
		return feedGap(b, line)

	case "image":
		if line.Image == "" {
			return nil
		}
		img, err := decodeImage(line.Image)
		if err != nil {
			return fmt.Errorf("una imagen del ticket no se pudo leer: debe ser PNG, JPEG o GIF en base64")
		}
		alignOf(b, line.Align, "center")
		b.ImageFit(img, imageWidth(res.Doc.PaperWidth, res.Scale)).Line()
		return feedGap(b, line)

	case "rule":
		ch := line.Rule
		if ch == "" {
			ch = "-"
		}
		inner := cols
		if border {
			inner = cols - 2
		}
		rule := strings.Repeat(string([]rune(ch)[0]), inner)
		if border {
			rule = "│" + rule + "│"
		}
		b.AlignLeft().TextLine(rule)
		return feedGap(b, line)

	case "feed":
		n := line.Feed
		if n <= 0 {
			n = 1
		}
		b.Feed(n)
		return nil
	}

	// Texto.
	text := line.Text
	r := []rune(text)
	if line.Box {
		drawBoxLine(b, line, r, cols, border)
		return nil
	}

	ml, mr := clampMin0(line.ML), clampMin0(line.MR)
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
			text = strings.Repeat(" ", ml+left) + string(r) + strings.Repeat(" ", pad-left+mr)
		case "right":
			text = strings.Repeat(" ", ml+inner-len(r)) + string(r) + strings.Repeat(" ", mr)
		default:
			text = strings.Repeat(" ", ml) + string(r) + strings.Repeat(" ", inner-len(r)+mr)
		}
		text = "│" + text + "│"
	} else {
		text = strings.Repeat(" ", ml) + text + strings.Repeat(" ", mr)
		alignOf(b, line.Align, "left")
	}

	applyScale(b, line)
	b.Bold(line.Bold).Underline(line.Underline).Inverted(line.Invert).TextLine(text)
	b.TextScale(1, 1).Font("a").Bold(false).Underline(false).Inverted(false)
	return feedGap(b, line)
}

// maxTicketCols acota los valores que se cuentan en caracteres. Sin tope,
// un margin_left de 1.500.000.000 hacia que strings.Repeat pidiera 1,5 GB y
// el proceso moria con un fatal error del runtime, que recover no atrapa:
// 55 bytes de JSON tumbaban el agente de impresion.
const maxTicketCols = 200

// maxTicketGap acota las lineas en blanco entre elementos.
const maxTicketGap = 100

func clampMin0(v int) int {
	return clampCols(v)
}

func clampCols(v int) int {
	if v < 0 {
		return 0
	}
	if v > maxTicketCols {
		return maxTicketCols
	}
	return v
}

func clampGap(v int) int {
	if v < 0 {
		return 0
	}
	if v > maxTicketGap {
		return maxTicketGap
	}
	return v
}

func alignOf(b *escpos.Builder, align, def string) {
	if align == "" {
		align = def
	}
	switch strings.ToLower(align) {
	case "center":
		b.AlignCenter()
	case "right":
		b.AlignRight()
	default:
		b.AlignLeft()
	}
}

func feedGap(b *escpos.Builder, line ticketLine) error {
	for i := 0; i < clampGap(line.Gap); i++ {
		b.Line()
	}
	return nil
}

// applyScale admite tanto el multiplicador exacto (1 a 8) como los nombres de
// siempre. El exacto manda.
func applyScale(b *escpos.Builder, line ticketLine) {
	if line.ScaleW > 0 || line.ScaleH > 0 {
		w, h := line.ScaleW, line.ScaleH
		if w <= 0 {
			w = 1
		}
		if h <= 0 {
			h = 1
		}
		b.TextScale(w, h)
		return
	}
	switch strings.ToLower(line.Size) {
	case "double":
		b.TextScale(2, 2)
	case "wide":
		b.TextScale(2, 1)
	case "tall":
		b.TextScale(1, 2)
	case "small":
		b.TextScale(1, 1).Font("b")
	default:
		b.TextScale(1, 1).Font("a")
	}
}

func drawBoxLine(b *escpos.Builder, line ticketLine, r []rune, cols int, border bool) {
	inner := cols
	if border {
		inner = cols - 2
	}
	ml := clampCols(line.ML)
	mr := clampCols(line.MR)
	// El texto comparte el ancho interior con los margenes y con el marco
	// ("│  " + " │" = 4 caracteres), asi que se trunca contando ya los
	// margenes. Truncar antes de sumarlos dejaba la linea mas ancha que la
	// caja y hacia que el relleno de abajo saliera negativo.
	maxText := inner - 4 - ml - mr
	if maxText < 1 {
		maxText = 1
		ml, mr = 0, 0
	}
	if len(r) > maxText {
		r = r[:maxText]
	}
	if ml+mr > 0 {
		r = append([]rune(strings.Repeat(" ", ml)), r...)
		r = append(r, []rune(strings.Repeat(" ", mr))...)
	}
	wall := len(r) + 4
	if wall+2 > inner {
		wall = inner - 2
	}
	if wall < len(r)+2 {
		wall = len(r) + 2
	}
	// El relleno se reparte a los dos lados para que el texto quede centrado
	// dentro del recuadro, y suma exactamente el ancho del marco: antes la
	// linea de contenido salia un caracter mas ancha que los bordes.
	pad := wall - len(r) - 2
	if pad < 0 {
		pad = 0
	}
	left := pad / 2
	right := pad - left
	topLine := "┌" + strings.Repeat("─", wall) + "┐"
	ctLine := "│ " + strings.Repeat(" ", left) + string(r) + strings.Repeat(" ", right) + " │"
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
	for i := 0; i < clampGap(line.Gap); i++ {
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

// templateName normaliza el nombre: quita la extension y se queda con la
// plantilla conocida. Antes se usaba strings.Contains en cadena, asi que
// "factura-qr" caia en la rama "qr" porque se evaluaba antes.
func templateName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimSuffix(name, ".html")
	for _, known := range nativeTemplates {
		if name == known {
			return known
		}
	}
	for _, known := range nativeTemplates {
		if strings.HasPrefix(name, known) {
			return known
		}
	}
	return "factura"
}

func buildNativeTemplate(req templateRequest, res resolved) *escpos.Builder {
	data := req.Data
	template := templateName(req.Template)
	cols := res.Doc.Columns()
	empresa := cleanText(dataString(data, "empresa", "COLLATECH"))
	cliente := cleanText(dataString(data, "cliente", "Cliente Demo"))
	total := cleanText(dataString(data, "total", "0.00"))
	mensaje := cleanText(dataString(data, "mensaje", "Gracias por su compra"))
	qr := dataString(data, "qr", "")
	barcode := dataString(data, "barcode", "")
	items := dataItems(data)

	b := escpos.Begin(res.Doc)
	b.AlignCenter().Bold(true).TextScale(2, 2).TextLine(empresa).TextScale(1, 1).Bold(false)

	switch template {
	case "recibo":
		b.AlignCenter().TextLine("RECIBO")
		b.AlignLeft().TextLine("Cliente: " + cliente)
		b.TextLine(strings.Repeat("-", cols))
		b.Bold(true).TextLine(padBoth("TOTAL PAGADO", total, cols)).Bold(false)
	case "comanda":
		b.AlignCenter().TextLine("COMANDA")
		b.AlignLeft().TextLine("Mesa/Cliente: " + cliente)
		b.TextLine(strings.Repeat("-", cols))
		for _, item := range items {
			b.TextLine("- " + cleanText(item.Name))
		}
	case "texto":
		b.AlignLeft().TextLine(mensaje)
		b.TextLine(time.Now().Format("2006-01-02 15:04:05"))
	case "qr":
		b.AlignCenter().TextLine("QR DE PRUEBA")
		if qr == "" {
			qr = "https://kollatek.com"
		}
		b.QRWith(qr, escpos.QROptions{PaperWidth: res.Doc.PaperWidth}).Line()
	case "imagen":
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
				{Text: "PRODUCTO", Width: cols - 16, Align: "left"},
				{Text: "PRECIO", Width: 12, Align: "right"},
			},
			Rows: rows,
		}, cols, false, 0, 0)
		b.TextLine(strings.Repeat("-", cols))
		b.Bold(true).TextLine(padBoth("TOTAL", total, cols)).Bold(false)
	}

	if qr != "" && template != "qr" {
		b.AlignCenter().Line().QRWith(qr, escpos.QROptions{PaperWidth: res.Doc.PaperWidth}).Line()
	}
	if barcode != "" {
		b.AlignCenter().BarcodeWith(barcode, escpos.BarcodeOptions{PaperWidth: res.Doc.PaperWidth}).Line()
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
	// Se cuenta en runas, no en bytes: con len() un "TOTAL ARTICULOS" con
	// tilde desalineaba la columna, y el recorte podia partir un caracter
	// UTF-8 por la mitad.
	lr := []rune(left)
	rr := []rune(right)
	maxLeft := width - len(rr) - 1
	if maxLeft < 1 {
		return left + " " + right
	}
	if len(lr) > maxLeft {
		lr = lr[:maxLeft]
	}
	spaces := width - len(lr) - len(rr)
	if spaces < 1 {
		spaces = 1
	}
	return string(lr) + strings.Repeat(" ", spaces) + string(rr)
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
