// Package layout compone un bloque de ticket en dos dimensiones y lo
// convierte en una imagen lista para la impresora.
//
// Por que existe: una impresora ESC/POS es una impresora de LINEAS. El texto
// fluye de arriba abajo y el comando nativo de QR imprime y avanza el papel,
// asi que no hay forma de poner un QR al costado de un texto con comandos
// nativos.
//
// La salida de este paquete es una imagen, de modo que dentro de un bloque se
// puede colocar lo que sea donde sea: un QR pequeno a la derecha del total,
// el logo junto a la direccion, columnas con bordes, tablas con el aspecto
// que uno quiera.
//
// Lo que cuesta: un bloque de 576x200 son unos 14 KB de raster frente a los
// ~200 bytes del mismo texto en nativo. Por USB o red no se nota; por serie
// lento si. Por eso el agente usa nativo para el texto corriente y deja este
// paquete para las zonas que necesitan maquetacion de verdad.
package layout

import (
	"fmt"
	"image"
	"strings"
)

// Layout es un bloque: una pila de filas, cada una con sus columnas.
type Layout struct {
	// Width es el ancho util en puntos. Lo fija el agente segun el papel.
	Width int `json:"-"`
	// Gap es la separacion VERTICAL entre filas, en puntos. Es un puntero
	// para poder distinguir "no indicado" de "cero": con un int normal no
	// habia forma de pedir filas pegadas.
	Gap *int `json:"gap"`
	// Padding es el margen interior del bloque.
	Padding int `json:"padding"`
	// Border dibuja un marco alrededor de todo el bloque.
	Border bool  `json:"border"`
	Rows   []Row `json:"rows"`
}

// Row es una franja horizontal dividida en columnas.
type Row struct {
	Cols []Col `json:"cols"`
	// Gap es la separacion HORIZONTAL entre las columnas de esta fila, en
	// puntos. Omitirlo usa 6; cero las pega.
	//
	// Antes este mismo campo servia tambien de hueco vertical despues de la
	// fila, asi que separar dos columnas hacia crecer el alto del bloque.
	Gap *int `json:"gap"`
	// Align vertical de las columnas mas bajas: top, middle o bottom.
	Align string `json:"align"`
	// Border dibuja un marco alrededor de la fila.
	Border bool `json:"border"`
	// MinHeight fuerza una altura minima en puntos.
	MinHeight int `json:"min_height"`
}

// Col es una columna dentro de una fila.
type Col struct {
	// Weight reparte el ancho sobrante entre las columnas sin Dots. Con 0 se
	// entiende 1.
	Weight int `json:"weight"`
	// Dots fija el ancho exacto en puntos y manda sobre Weight. Es lo que se
	// usa para un QR pequeno al costado.
	Dots int `json:"dots"`
	// Align horizontal del contenido: left, center o right.
	Align string `json:"align"`
	// Pad es el margen interior de la columna.
	Pad int `json:"pad"`
	// Border dibuja un marco alrededor de la columna.
	Border bool   `json:"border"`
	Items  []Item `json:"items"`
}

// Item es un elemento dentro de una columna.
type Item struct {
	// Type: text (por defecto), qr, barcode, image, rule o space.
	Type string `json:"type"`

	Text string `json:"text"`
	// Size por nombre: xs, s, m (por defecto), l, xl, xxl.
	Size string `json:"size"`
	// SizePx fija la altura de la fuente en puntos y manda sobre Size.
	SizePx int  `json:"size_px"`
	Bold   bool `json:"bold"`
	// Mono usa fuente de ancho fijo, util para alinear importes.
	Mono bool `json:"mono"`
	// Invert pinta blanco sobre negro.
	Invert bool `json:"invert"`
	// Align del propio elemento; si falta, hereda el de la columna.
	Align string `json:"align"`

	// QR y su tamano de modulo en puntos (0 = ajustar al ancho disponible).
	QR     string `json:"qr"`
	QRSize int    `json:"qr_size"`
	QREC   string `json:"qr_ec"`

	// Barcode y sus medidas.
	Barcode     string `json:"barcode"`
	BarcodeType string `json:"barcode_type"`
	BarHeight   int    `json:"bar_height"`

	// Image en base64 o data URI.
	Image string `json:"image"`

	// Height en puntos, para space y rule.
	Height int `json:"height"`

	// Gap debajo del elemento, en puntos.
	Gap int `json:"gap"`
}

// Defaults razonables para un ticket a 203 ppp.
const (
	defaultGap     = 6
	defaultRuleH   = 2
	minColumnWidth = 16
	borderWidth    = 2
)

const (
	// MaxHeight acota la altura del bloque. Un raster muy alto puede
	// desbordar el buffer de las termicas baratas y gastar papel sin querer.
	MaxHeight = 4000

	// MaxWidth acota el ancho. La termica mas ancha de ticket son 576
	// puntos; se deja margen para formatos raros, pero sin tope el lienzo
	// podia llegar a cientos de MB y dejar el agente sin memoria.
	MaxWidth = 1024
)

// Topes de los valores que llegan por JSON. Acotar solo por abajo no basta:
// un bar_height de 2^30 hacia que image.NewGray pidiera 566 GB y el proceso
// moria con "fatal error: out of memory", que es un error del runtime y NO
// lo atrapa el recover del servidor. Una sola peticion tumbaba el agente.
const (
	maxPadding   = 200
	maxGap       = 200
	maxWeight    = 100
	maxQRModule  = 16
	maxBarHeight = 255
	maxFontPx    = 200
)

// normalize acota los valores que llegan por JSON. Sin esto, un numero fuera
// de rango no fallaba: hacia cosas peores y en silencio. Un `pad` negativo
// daba a la columna mas ancho util que la propia columna, asi que el
// contenido se dibujaba fuera de su caja; un `padding` negativo dejaba el
// bloque en una imagen de un punto de alto, perdiendo todo el ticket; y los
// valores enormes reventaban el proceso.
// Devuelve una COPIA acotada. No se toca el original: Layout se pasa por
// valor pero Rows es un slice, asi que mutarlo en sitio escribia en los datos
// de quien llama. Componer dos veces el mismo diseno a la vez era una carrera
// de datos, y ademas el bloque quedaba modificado despues de imprimirlo.
func (l Layout) normalized() Layout {
	out := l
	out.Padding = clamp(l.Padding, 0, maxPadding)
	out.Gap = clampPtr(l.Gap, maxGap)

	out.Rows = make([]Row, len(l.Rows))
	for i, r := range l.Rows {
		fila := r
		fila.MinHeight = clamp(r.MinHeight, 0, MaxHeight)
		fila.Gap = clampPtr(r.Gap, maxGap)

		fila.Cols = make([]Col, len(r.Cols))
		for j, c := range r.Cols {
			col := c
			col.Pad = clamp(c.Pad, 0, maxPadding)
			col.Dots = clamp(c.Dots, 0, MaxWidth)
			col.Weight = clamp(c.Weight, 0, maxWeight)

			col.Items = make([]Item, len(c.Items))
			for k, it := range c.Items {
				elem := it
				elem.Gap = clamp(it.Gap, 0, maxGap)
				elem.Height = clamp(it.Height, 0, MaxHeight)
				elem.SizePx = clamp(it.SizePx, 0, maxFontPx)
				elem.QRSize = clamp(it.QRSize, 0, maxQRModule)
				elem.BarHeight = clamp(it.BarHeight, 0, maxBarHeight)
				col.Items[k] = elem
			}
			fila.Cols[j] = col
		}
		out.Rows[i] = fila
	}
	return out
}

// clampPtr acota un puntero sin tocar el original.
func clampPtr(p *int, max int) *int {
	if p == nil {
		return nil
	}
	v := clamp(*p, 0, max)
	return &v
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// Render compone el bloque y devuelve la imagen en blanco y negro.
func Render(l Layout) (image.Image, error) {
	l = l.normalized()
	if l.Width <= 0 {
		l.Width = 576
	}
	if l.Width > MaxWidth {
		return nil, fmt.Errorf("el bloque mide %d puntos de ancho, el maximo son %d", l.Width, MaxWidth)
	}
	gapBloque := gapOr(l.Gap, defaultGap)
	inner := l.Width - 2*l.Padding
	if l.Border {
		inner -= 2 * borderWidth
	}
	if inner < minColumnWidth {
		return nil, fmt.Errorf("el bloque es demasiado estrecho: %d puntos utiles", inner)
	}

	// Las fuentes son de este render y de nadie mas: compartirlas entre
	// goroutines era una carrera de datos.
	fs := newFontSet()
	defer fs.close()

	// Primera pasada: medir cada fila para saber el alto total.
	medidas := make([]rowLayout, 0, len(l.Rows))
	total := 0
	for i, row := range l.Rows {
		m, err := measureRow(fs, row, inner)
		if err != nil {
			return nil, fmt.Errorf("fila %d: %w", i+1, err)
		}
		medidas = append(medidas, m)
		total += m.height
		if i < len(l.Rows)-1 {
			total += gapBloque
		}
		// El alto se comprueba sobre la marcha: sumarlo todo y mirar al
		// final podia desbordar el entero y saltarse el limite.
		if total > MaxHeight {
			return nil, fmt.Errorf("el bloque pasa de %d puntos de alto", MaxHeight)
		}
	}

	alto := total + 2*l.Padding
	if l.Border {
		alto += 2 * borderWidth
	}
	if alto < 1 {
		alto = 1
	}
	if alto > MaxHeight {
		return nil, fmt.Errorf("el bloque mide %d puntos de alto, el maximo son %d", alto, MaxHeight)
	}

	canvas := newCanvas(l.Width, alto)
	y := l.Padding
	x := l.Padding
	if l.Border {
		canvas.rect(0, 0, l.Width, alto, borderWidth)
		y += borderWidth
		x += borderWidth
	}
	for i, row := range l.Rows {
		drawRow(fs, canvas, medidas[i], row, x, y, inner)
		y += medidas[i].height
		if i < len(l.Rows)-1 {
			y += gapBloque
		}
	}
	return canvas.gray(), nil
}

// rowLayout guarda lo medido de una fila para no recalcularlo al dibujar.
type rowLayout struct {
	height  int
	widths  []int
	heights []int
	items   [][]measuredItem
}

type measuredItem struct {
	item   Item
	height int
	// rendered guarda lo ya compuesto (QR, imagen, codigo de barras) para no
	// generarlo dos veces.
	rendered image.Image
	// lines son las lineas de texto ya partidas al ancho de la columna.
	lines []string
	face  *faceKey
}

func measureRow(fs *fontSet, row Row, avail int) (rowLayout, error) {
	n := len(row.Cols)
	if n == 0 {
		return rowLayout{height: row.MinHeight}, nil
	}
	gap := gapOr(row.Gap, defaultGap)
	libre := avail - gap*(n-1)
	if row.Border {
		libre -= 2 * borderWidth
	}
	// Si no caben ni los minimos, se recorta el hueco antes que desbordar el
	// papel: subir el presupuesto por encima de lo disponible dejaba la
	// ultima columna fuera del ticket, sin avisar.
	if libre < n*minColumnWidth {
		gap = 0
		libre = avail
		if row.Border {
			libre -= 2 * borderWidth
		}
	}
	if libre < n {
		libre = n
	}

	// Primero las columnas de ancho fijo, luego se reparte lo que queda.
	widths := make([]int, n)
	restante := libre
	pesoTotal := 0
	for i, c := range row.Cols {
		if c.Dots > 0 {
			w := c.Dots
			if w > restante {
				w = restante
			}
			widths[i] = w
			restante -= w
			continue
		}
		pesoTotal += pick(c.Weight, 1)
	}
	if pesoTotal > 0 {
		base := restante / pesoTotal
		usado, ultimo := 0, -1
		for i, c := range row.Cols {
			if c.Dots > 0 {
				continue
			}
			w := base * pick(c.Weight, 1)
			widths[i] = w
			usado += w
			ultimo = i
		}
		// El redondeo sobrante va a la ultima columna flexible, para que la
		// fila ocupe el ancho exacto.
		if ultimo >= 0 {
			widths[ultimo] += restante - usado
		}
	}

	out := rowLayout{widths: widths, heights: make([]int, n), items: make([][]measuredItem, n)}
	for i, c := range row.Cols {
		w := widths[i] - 2*c.Pad
		if c.Border {
			w -= 2 * borderWidth
		}
		if w < 1 {
			w = 1
		}
		medidos, alto, err := measureItems(fs, c, w)
		if err != nil {
			return out, fmt.Errorf("columna %d: %w", i+1, err)
		}
		alto += 2 * c.Pad
		if c.Border {
			alto += 2 * borderWidth
		}
		out.items[i] = medidos
		out.heights[i] = alto
		if alto > out.height {
			out.height = alto
		}
	}
	if row.Border {
		out.height += 2 * borderWidth
	}
	if row.MinHeight > out.height {
		out.height = row.MinHeight
	}
	return out, nil
}

func measureItems(fs *fontSet, c Col, w int) ([]measuredItem, int, error) {
	out := make([]measuredItem, 0, len(c.Items))
	total := 0
	for j, it := range c.Items {
		m, err := measureItem(fs, it, w)
		if err != nil {
			return nil, 0, fmt.Errorf("elemento %d: %w", j+1, err)
		}
		out = append(out, m)
		total += m.height + it.Gap
	}
	return out, total, nil
}

// gapOr distingue "no indicado" (nil) de "cero".
func gapOr(p *int, def int) int {
	if p == nil {
		return def
	}
	if *p < 0 {
		return 0
	}
	return *p
}

func pick(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

// itemType deduce el tipo cuando no se indica, a partir del campo que venga
// relleno: asi `{"qr": "..."}` ya es un QR sin tener que escribir el type.
func itemType(it Item) string {
	if t := strings.ToLower(strings.TrimSpace(it.Type)); t != "" {
		return t
	}
	switch {
	case it.QR != "":
		return "qr"
	case it.Barcode != "":
		return "barcode"
	case it.Image != "":
		return "image"
	}
	return "text"
}

func alignOf(values ...string) string {
	for _, v := range values {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "center", "centro":
			return "center"
		case "right", "derecha":
			return "right"
		case "left", "izquierda":
			return "left"
		}
	}
	return "left"
}
