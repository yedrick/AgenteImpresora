package render

import "strings"

// renderTable dibuja la tabla acumulada ajustandola al ancho del papel.
//
// El reparto de anchos tiene en cuenta lo que mide cada celda y respeta el
// atributo width cuando se indica, de forma que una columna de importes no
// se come el espacio del nombre del articulo.
func (h *HTMLToESCPOS) renderTable() {
	rows, headers := h.tableRows, h.rowHeader
	h.tableRows, h.rowHeader = nil, nil
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

	// Con pocas columnas el marco se ve bien; con muchas se queda sin sitio
	// para el texto y se imprime sin el.
	overhead := numCols*3 + 1
	border := numCols <= 4 && overhead+numCols*4 <= h.cols
	avail := h.cols - (numCols - 1)
	if border {
		avail = h.cols - overhead
	}
	if avail < numCols {
		avail = numCols
	}

	widths := h.columnWidths(rows, numCols, avail)

	h.builder.AlignLeft()
	if border {
		h.builder.TextLine(tableRule(widths, "┌", "┬", "┐"))
	}
	for ri, row := range rows {
		cells := make([]string, numCols)
		for i := 0; i < numCols; i++ {
			txt := ""
			if i < len(row) {
				txt = row[i]
			}
			// La ultima columna se alinea a la derecha: en un ticket suele
			// ser el importe.
			if i == numCols-1 && numCols > 1 {
				cells[i] = padLeft(txt, widths[i])
			} else {
				cells[i] = padRight(txt, widths[i])
			}
		}
		esHeader := ri < len(headers) && headers[ri]
		if esHeader {
			h.builder.Bold(true)
		}
		if border {
			h.builder.TextLine("│ " + strings.Join(cells, " │ ") + " │")
		} else {
			h.builder.TextLine(strings.Join(cells, " "))
		}
		if esHeader {
			h.builder.Bold(false)
		}
		h.hasContent = true

		if ri == 0 && len(rows) > 1 && (esHeader || border) {
			if border {
				h.builder.TextLine(tableRule(widths, "├", "┼", "┤"))
			} else if esHeader {
				h.builder.TextLine(strings.Repeat("-", h.cols))
			}
		}
	}
	if border {
		h.builder.TextLine(tableRule(widths, "└", "┴", "┘"))
	}
}

// columnWidths reparte el ancho disponible: primero lo que pida el atributo
// width, luego lo que necesite el contenido, y si no cabe se recorta en
// proporcion empezando por las columnas mas anchas.
func (h *HTMLToESCPOS) columnWidths(rows [][]string, numCols, avail int) []int {
	widths := make([]int, numCols)
	for i := range widths {
		widths[i] = 1
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= numCols {
				break
			}
			if n := len([]rune(cell)); n > widths[i] {
				widths[i] = n
			}
		}
	}
	// El atributo width del HTML manda sobre el contenido.
	for i := 0; i < numCols && i < len(h.colWidths); i++ {
		if h.colWidths[i] > 0 {
			widths[i] = h.colWidths[i]
		}
	}

	total := 0
	for _, w := range widths {
		total += w
	}
	if total <= avail {
		// Lo que sobre va a la primera columna, que suele ser la descripcion.
		widths[0] += avail - total
		return widths
	}

	// No cabe: se recorta de la mas ancha hasta que entre, para no dejar
	// ninguna columna inservible.
	for total > avail {
		mayor := 0
		for i, w := range widths {
			if w > widths[mayor] {
				mayor = i
			}
		}
		if widths[mayor] <= 1 {
			break
		}
		widths[mayor]--
		total--
	}
	return widths
}

func tableRule(widths []int, left, mid, right string) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = strings.Repeat("─", w+2)
	}
	return left + strings.Join(parts, mid) + right
}

func padRight(text string, width int) string {
	r := []rune(text)
	if len(r) > width {
		r = r[:width]
	}
	return string(r) + strings.Repeat(" ", width-len(r))
}

func padLeft(text string, width int) string {
	r := []rune(text)
	if len(r) > width {
		r = r[:width]
	}
	return strings.Repeat(" ", width-len(r)) + string(r)
}
