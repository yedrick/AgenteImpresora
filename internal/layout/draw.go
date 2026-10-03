package layout

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

// measureItem calcula cuanto ocupa un elemento y deja compuesto lo que haga
// falta, para no generarlo dos veces al dibujar.
func measureItem(fs *fontSet, it Item, width int) (measuredItem, error) {
	m := measuredItem{item: it}

	// Un elemento recien anadido y todavia sin contenido no es un error: en
	// el disenador, tratarlo como tal borraba la vista previa del ticket
	// entero por un hueco que el usuario aun no habia rellenado. Se queda en
	// nada y ya.
	switch itemType(it) {
	case "qr":
		if strings.TrimSpace(it.QR) == "" {
			return m, nil
		}
	case "barcode":
		if strings.TrimSpace(it.Barcode) == "" {
			return m, nil
		}
	case "image":
		if strings.TrimSpace(it.Image) == "" {
			return m, nil
		}
	}

	switch itemType(it) {
	case "space":
		m.height = pick(it.Height, defaultGap)

	case "rule":
		m.height = pick(it.Height, defaultRuleH)

	case "qr":
		// Un QR al costado suele querer ser pequeno: si no se indica tamano
		// se ajusta al ancho de su columna, que es justo lo que lo hace util
		// en una maquetacion de dos columnas.
		img, err := renderQR(it.QR, it.QRSize, width, it.QREC)
		if err != nil {
			return m, err
		}
		m.rendered = img
		m.height = img.Bounds().Dy()

	case "barcode":
		img, err := renderBarcode(it.Barcode, it.BarHeight, 0, width)
		if err != nil {
			return m, err
		}
		m.rendered = img
		m.height = img.Bounds().Dy()

	case "image":
		img, err := decodeImage(it.Image)
		if err != nil {
			return m, fmt.Errorf("no se pudo leer la imagen: %w", err)
		}
		escalada := fitImage(img, width, pick(it.Height, 0))
		m.rendered = escalada
		m.height = escalada.Bounds().Dy()

	default: // texto
		k := keyFor(it)
		m.face = k
		m.lines = fs.wrapText(k, it.Text, width)
		m.height = len(m.lines) * fs.lineHeight(k)
	}
	return m, nil
}

func drawRow(fs *fontSet, c *canvas, m rowLayout, row Row, x, y, avail int) {
	if len(row.Cols) == 0 {
		return
	}
	if row.Border {
		c.rect(x, y, avail, m.height, borderWidth)
		x += borderWidth
		y += borderWidth
	}
	gap := gapOr(row.Gap, defaultGap)
	alturaFila := m.height
	if row.Border {
		alturaFila -= 2 * borderWidth
	}

	cx := x
	for i, col := range row.Cols {
		w := m.widths[i]
		// Alineacion vertical de las columnas mas bajas que la fila.
		dy := 0
		switch strings.ToLower(row.Align) {
		case "middle", "center":
			dy = (alturaFila - m.heights[i]) / 2
		case "bottom":
			dy = alturaFila - m.heights[i]
		}
		if dy < 0 {
			dy = 0
		}
		drawCol(fs, c, col, m.items[i], cx, y+dy, w, m.heights[i])
		cx += w + gap
	}
}

func drawCol(fs *fontSet, c *canvas, col Col, items []measuredItem, x, y, w, h int) {
	if col.Border {
		c.rect(x, y, w, h, borderWidth)
		x += borderWidth
		y += borderWidth
		w -= 2 * borderWidth
	}
	izq, der, arr, _ := col.lados()
	x += izq
	y += arr
	w -= izq + der
	if w < 1 {
		w = 1
	}

	cy := y
	for _, m := range items {
		drawItem(fs, c, m, col, x, cy, w)
		cy += m.height + m.item.Gap
	}
}

func drawItem(fs *fontSet, c *canvas, m measuredItem, col Col, x, y, w int) {
	align := alignOf(m.item.Align, col.Align)

	switch itemType(m.item) {
	case "space":
		return

	case "rule":
		c.fill(x, y, w, pick(m.item.Height, defaultRuleH), 0)
		return

	case "qr", "barcode", "image":
		if m.rendered == nil {
			return // sin contenido todavia
		}
		iw := m.rendered.Bounds().Dx()
		dx := 0
		switch align {
		case "center":
			dx = (w - iw) / 2
		case "right":
			dx = w - iw
		}
		if dx < 0 {
			dx = 0
		}
		// Recortado al ancho de la columna: un simbolo mas ancho se metia en
		// la columna de al lado.
		c.blitClip(m.rendered, x+dx, y, w-dx)
		return
	}

	// Texto.
	alto := fs.lineHeight(m.face)
	if m.item.Invert {
		// El resaltado se pinta primero y el texto encima, invirtiendo todo
		// el bloque al final.
		c.fill(x, y, w, alto*len(m.lines), 255)
	}
	cy := y
	for _, linea := range m.lines {
		fs.drawText(c, m.face, linea, x, cy, w, align)
		cy += alto
	}
	if m.item.Invert {
		c.invert(x, y, w, alto*len(m.lines))
	}
}

// maxImagePixels acota la imagen antes de descomprimirla: un PNG en blanco
// de 20000x20000 ocupa 400 KB comprimido y cientos de MB en memoria.
const maxImagePixels = 12_000_000

func decodeImage(src string) (image.Image, error) {
	src = strings.TrimSpace(src)
	if i := strings.Index(src, ","); strings.HasPrefix(src, "data:") && i >= 0 {
		src = src[i+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return nil, fmt.Errorf("no es base64 valido")
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

// fitImage reduce la imagen para que quepa, sin ampliarla nunca: ampliar un
// logo solo lo emborrona.
func fitImage(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	escala := 1.0
	if maxW > 0 && w > maxW {
		escala = float64(maxW) / float64(w)
	}
	if maxH > 0 && float64(h)*escala > float64(maxH) {
		escala = float64(maxH) / float64(h)
	}
	if escala >= 1 {
		return toGray(src)
	}
	dw := int(float64(w) * escala)
	dh := int(float64(h) * escala)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}

	// Promedio por bloques: tomar el pixel mas cercano deja dientes de
	// sierra en un logo con texto.
	out := image.NewGray(image.Rect(0, 0, dw, dh))
	gris := toGray(src)
	for y := 0; y < dh; y++ {
		sy0 := y * h / dh
		sy1 := (y + 1) * h / dh
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < dw; x++ {
			sx0 := x * w / dw
			sx1 := (x + 1) * w / dw
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var suma, n uint32
			for sy := sy0; sy < sy1 && sy < h; sy++ {
				fila := gris.Pix[sy*gris.Stride:]
				for sx := sx0; sx < sx1 && sx < w; sx++ {
					suma += uint32(fila[sx])
					n++
				}
			}
			if n == 0 {
				n = 1
			}
			out.Pix[y*out.Stride+x] = uint8(suma / n)
		}
	}
	return out
}

func toGray(src image.Image) *image.Gray {
	if g, ok := src.(*image.Gray); ok {
		return g
	}
	b := src.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bb, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Se aplana sobre blanco: el papel del ticket.
			inv := 0xffff - a
			l := (299*(r+inv) + 587*(g+inv) + 114*(bb+inv)) / 1000 >> 8
			if l > 255 {
				l = 255
			}
			out.Pix[y*out.Stride+x] = uint8(l)
		}
	}
	return out
}
