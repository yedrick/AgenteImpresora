package escpos

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
)

type Builder struct {
	buf bytes.Buffer
}

func New() *Builder { return &Builder{} }

const (
	// CutFeedLines es el avance antes del corte. La cuchilla esta varias
	// lineas por encima del cabezal, asi que avanzar solo 1 cortaba la ultima
	// linea del ticket.
	CutFeedLines = 4

	// MaxBarcodeLen es el limite del byte de longitud de "GS k".
	MaxBarcodeLen = 255
	// MaxQRLen es la capacidad maxima de un QR modelo 2 en modo byte.
	MaxQRLen = 2953
	// rasterBandRows trocea la imagen: muchas termicas desbordan el buffer si
	// se les manda un unico "GS v 0" muy alto.
	rasterBandRows = 128
)

func (b *Builder) Initialize() *Builder {
	b.buf.Write([]byte{0x1b, 0x40})
	b.CodePageCP850()
	return b
}

func (b *Builder) CodePageCP850() *Builder {
	b.buf.Write([]byte{0x1b, 0x74, 0x02})
	return b
}

func (b *Builder) Text(text string) *Builder {
	b.buf.Write(EncodeCP850(text))
	return b
}

func (b *Builder) TextLine(text string) *Builder {
	b.buf.Write(EncodeCP850(text))
	b.Line()
	return b
}

func (b *Builder) Line() *Builder {
	b.buf.WriteByte('\n')
	return b
}

func (b *Builder) AlignLeft() *Builder   { return b.align(0) }
func (b *Builder) AlignCenter() *Builder { return b.align(1) }
func (b *Builder) AlignRight() *Builder  { return b.align(2) }

func (b *Builder) align(mode byte) *Builder {
	b.buf.Write([]byte{0x1b, 0x61, mode})
	return b
}

func (b *Builder) Bold(on bool) *Builder {
	return b.onOff([]byte{0x1b, 0x45}, on)
}

func (b *Builder) Underline(on bool) *Builder {
	return b.onOff([]byte{0x1b, 0x2d}, on)
}

func (b *Builder) DoubleSize(on bool) *Builder {
	if on {
		b.buf.Write([]byte{0x1d, 0x21, 0x11})
	} else {
		b.buf.Write([]byte{0x1d, 0x21, 0x00})
	}
	return b
}

func (b *Builder) FontSize(size string) *Builder {
	switch strings.ToLower(size) {
	case "double":
		b.buf.Write([]byte{0x1d, 0x21, 0x11})
	case "wide":
		b.buf.Write([]byte{0x1d, 0x21, 0x10})
	case "tall":
		b.buf.Write([]byte{0x1d, 0x21, 0x01})
	case "small":
		b.buf.Write([]byte{0x1b, 0x4d, 0x01})
	case "normal":
		b.buf.Write([]byte{0x1d, 0x21, 0x00})
		b.buf.Write([]byte{0x1b, 0x4d, 0x00})
	}
	return b
}

func (b *Builder) Feed(lines int) *Builder {
	if lines < 1 {
		lines = 1
	}
	if lines > 255 {
		lines = 255
	}
	b.buf.Write([]byte{0x1b, 0x64, byte(lines)})
	return b
}

func (b *Builder) Cut() *Builder {
	b.buf.Write([]byte{0x1d, 0x56, 0x00})
	return b
}

func (b *Builder) DrawerKick() *Builder {
	b.buf.Write([]byte{0x1b, 0x70, 0x00, 0x19, 0xfa})
	return b
}

func (b *Builder) QR(data string) *Builder {
	payload := []byte(data)
	if len(payload) == 0 || len(payload) > MaxQRLen {
		return b
	}
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x04, 0x00, 0x31, 0x41, 0x32, 0x00})
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x43, 0x06})
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x45, 0x31})
	pLen := len(payload) + 3
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, byte(pLen % 256), byte(pLen / 256), 0x31, 0x50, 0x30})
	b.buf.Write(payload)
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x51, 0x30})
	return b
}

type BarcodeType byte

const (
	BarcodeUPCA    BarcodeType = 65
	BarcodeUPCE    BarcodeType = 66
	BarcodeEAN13   BarcodeType = 67
	BarcodeEAN8    BarcodeType = 68
	BarcodeCode39  BarcodeType = 69
	BarcodeCode93  BarcodeType = 72
	BarcodeCode128 BarcodeType = 73
	BarcodePDF417  BarcodeType = 0
)

func (b *Builder) Barcode(data string) *Builder {
	return b.BarcodeWithType(BarcodeCode128, data)
}

func (b *Builder) BarcodeWithType(kind BarcodeType, data string) *Builder {
	if kind == BarcodePDF417 {
		return b.PDF417(data)
	}
	payload := []byte(data)
	if kind == BarcodeCode128 && len(payload) > 0 && payload[0] != '{' {
		payload = append([]byte("{B"), payload...)
	}
	// byte(len(payload)) daba la vuelta con 256 bytes o mas: la impresora leia
	// unos pocos bytes y ejecutaba el resto del codigo como comandos.
	if len(payload) == 0 || len(payload) > MaxBarcodeLen {
		return b
	}
	b.buf.Write([]byte{0x1d, 0x48, 0x02})
	b.buf.Write([]byte{0x1d, 0x68, 0x64})
	b.buf.Write([]byte{0x1d, 0x77, 0x02})
	b.buf.Write([]byte{0x1d, 0x6b, byte(kind), byte(len(payload))})
	b.buf.Write(payload)
	return b
}

func (b *Builder) PDF417(data string) *Builder {
	payload := []byte(data)
	if len(payload) == 0 || len(payload) > MaxQRLen {
		return b
	}
	pLen := len(payload) + 3
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x30, 0x41, 0x00})
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, byte(pLen % 256), byte(pLen / 256), 0x30, 0x50, 0x30})
	b.buf.Write(payload)
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x30, 0x51, 0x30})
	return b
}

func (b *Builder) Image(img image.Image) *Builder {
	if img == nil {
		return b
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return b
	}
	widthBytes := (width + 7) / 8
	xL := byte(widthBytes % 256)
	xH := byte(widthBytes / 256)

	// Se emite una banda por cada rasterBandRows filas en vez de un unico
	// "GS v 0" con toda la imagen: las termicas con poco buffer cortaban o se
	// colgaban con logos altos.
	for y0 := 0; y0 < height; y0 += rasterBandRows {
		rows := rasterBandRows
		if y0+rows > height {
			rows = height - y0
		}
		b.buf.Write([]byte{0x1d, 0x76, 0x30, 0x00, xL, xH, byte(rows % 256), byte(rows / 256)})
		for y := y0; y < y0+rows; y++ {
			for xb := 0; xb < widthBytes; xb++ {
				var packed byte
				for bit := 0; bit < 8; bit++ {
					x := xb*8 + bit
					if x >= width {
						continue
					}
					if isDark(img.At(bounds.Min.X+x, bounds.Min.Y+y)) {
						packed |= 0x80 >> bit
					}
				}
				b.buf.WriteByte(packed)
			}
		}
	}
	return b
}

func (b *Builder) ImageFit(img image.Image, maxWidth int) *Builder {
	if img == nil {
		return b
	}
	return b.Image(PrepareImage(img, maxWidth))
}

func (b *Builder) Bytes() []byte {
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *Builder) onOff(prefix []byte, on bool) *Builder {
	v := byte(0)
	if on {
		v = 1
	}
	b.buf.Write(append(prefix, v))
	return b
}

func isDark(c color.Color) bool {
	r, g, bl, a := c.RGBA()
	if a == 0 {
		return false
	}
	luma := (299*r + 587*g + 114*bl) / 1000
	return luma < 0x8000
}

func PrepareImage(src image.Image, maxWidth int) image.Image {
	if maxWidth <= 0 {
		maxWidth = 384
	}
	src = trimBlankBorders(src)
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return src
	}

	targetWidth := maxWidth
	scale := float64(targetWidth) / float64(srcW)
	dstW := targetWidth
	dstH := int(math.Round(float64(srcH) * scale))
	if scale > 1 {
		dstW = srcW
		dstH = srcH
	}
	dstW = (dstW / 8) * 8
	if dstW < 8 {
		dstW = 8
	}
	if dstH < 1 {
		dstH = 1
	}
	scaled := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.Draw(scaled, scaled.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)

	for y := 0; y < dstH; y++ {
		for x := 0; x < dstW; x++ {
			sx := bounds.Min.X + int(float64(x)*float64(srcW)/float64(dstW))
			sy := bounds.Min.Y + int(float64(y)*float64(srcH)/float64(dstH))
			scaled.Set(x, y, flattenOnWhite(src.At(sx, sy)))
		}
	}

	return ditherImage(scaled)
}

// trimBlankBorders crops away fully white/transparent rows and columns
// around the edges of an image (e.g. padding baked into a logo PNG) so it
// doesn't print as dead space at the top of the ticket.
func trimBlankBorders(src image.Image) image.Image {
	bounds := src.Bounds()
	minX, minY, maxX, maxY := bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y

	isBlankRow := func(y int) bool {
		for x := minX; x < maxX; x++ {
			if !isBlankPixel(src.At(x, y)) {
				return false
			}
		}
		return true
	}
	isBlankCol := func(x int) bool {
		for y := minY; y < maxY; y++ {
			if !isBlankPixel(src.At(x, y)) {
				return false
			}
		}
		return true
	}

	top := minY
	for top < maxY && isBlankRow(top) {
		top++
	}
	bottom := maxY
	for bottom > top && isBlankRow(bottom-1) {
		bottom--
	}
	left := minX
	for left < maxX && isBlankCol(left) {
		left++
	}
	right := maxX
	for right > left && isBlankCol(right-1) {
		right--
	}

	if top == minY && bottom == maxY && left == minX && right == maxX {
		return src
	}
	if right <= left || bottom <= top {
		return src
	}

	cropped := image.NewRGBA(image.Rect(0, 0, right-left, bottom-top))
	draw.Draw(cropped, cropped.Bounds(), src, image.Point{X: left, Y: top}, draw.Src)
	return cropped
}

func isBlankPixel(c color.Color) bool {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return true
	}
	const threshold = 0xf000
	return r >= threshold && g >= threshold && b >= threshold
}

func flattenOnWhite(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	if a == 0xffff {
		return c
	}
	alpha := float64(a) / 65535.0
	rr := uint8(((float64(r)/257.0)*alpha + 255*(1-alpha)))
	gg := uint8(((float64(g)/257.0)*alpha + 255*(1-alpha)))
	bb := uint8(((float64(b)/257.0)*alpha + 255*(1-alpha)))
	return color.RGBA{R: rr, G: gg, B: bb, A: 255}
}

func ditherImage(src *image.RGBA) image.Image {
	bounds := src.Bounds()
	out := image.NewRGBA(bounds)
	w := bounds.Dx()
	h := bounds.Dy()
	pixels := make([]float64, w*h)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			// La division entera antes del float64 perdia precision justo
			// donde el difuminado la necesita.
			luma := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257.0
			pixels[(y-bounds.Min.Y)*w+(x-bounds.Min.X)] = luma
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*w + x
			old := pixels[i]
			newValue := 255.0
			if old < 128 {
				newValue = 0
			}
			err := old - newValue
			if newValue == 0 {
				out.Set(x, y, color.Black)
			} else {
				out.Set(x, y, color.White)
			}
			spreadError(pixels, w, h, x+1, y, err*7/16)
			spreadError(pixels, w, h, x-1, y+1, err*3/16)
			spreadError(pixels, w, h, x, y+1, err*5/16)
			spreadError(pixels, w, h, x+1, y+1, err*1/16)
		}
	}
	return out
}

func spreadError(pixels []float64, w, h, x, y int, err float64) {
	if x < 0 || x >= w || y < 0 || y >= h {
		return
	}
	i := y*w + x
	pixels[i] = math.Max(0, math.Min(255, pixels[i]+err))
}

func EncodeCP850(s string) []byte {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r >= 0 && r <= 127 {
			out = append(out, byte(r))
			continue
		}
		if b, ok := cp850[r]; ok {
			out = append(out, b)
			continue
		}
		if alt, ok := cp850Fallback[r]; ok {
			out = append(out, alt...)
			continue
		}
		out = append(out, '?')
	}
	return out
}

// cp850Fallback cubre caracteres tipograficos habituales que CP850 no tiene,
// para que no salgan como '?' en el ticket.
var cp850Fallback = map[rune]string{
	'\u2022': "*", '\u2026': "...", '\u2013': "-", '\u2014': "-",
	'\u2018': "'", '\u2019': "'", '\u201c': "\"", '\u201d': "\"",
	'\u20ac': "EUR", '\u2122': "(TM)", '\u2192': "->", '\u2713': "v",
	'\u00a0': " ",
}

var cp850 = map[rune]byte{
	'Ç': 0x80, 'ü': 0x81, 'é': 0x82, 'â': 0x83, 'ä': 0x84, 'à': 0x85, 'å': 0x86, 'ç': 0x87,
	'ê': 0x88, 'ë': 0x89, 'è': 0x8a, 'ï': 0x8b, 'î': 0x8c, 'ì': 0x8d, 'Ä': 0x8e, 'Å': 0x8f,
	'É': 0x90, 'æ': 0x91, 'Æ': 0x92, 'ô': 0x93, 'ö': 0x94, 'ò': 0x95, 'û': 0x96, 'ù': 0x97,
	'ÿ': 0x98, 'Ö': 0x99, 'Ü': 0x9a, 'ø': 0x9b, '£': 0x9c, 'Ø': 0x9d, '×': 0x9e, 'ƒ': 0x9f,
	'á': 0xa0, 'í': 0xa1, 'ó': 0xa2, 'ú': 0xa3, 'ñ': 0xa4, 'Ñ': 0xa5, 'ª': 0xa6, 'º': 0xa7,
	'¿': 0xa8, '®': 0xa9, '¬': 0xaa, '½': 0xab, '¼': 0xac, '¡': 0xad, '«': 0xae, '»': 0xaf,
	'Á': 0xb5, 'Â': 0xb6, 'À': 0xb7, '©': 0xb8, '╣': 0xb9, '║': 0xba, '╗': 0xbb, '╝': 0xbc,
	'¢': 0xbd, '¥': 0xbe, '┐': 0xbf, '└': 0xc0, '┴': 0xc1, '┬': 0xc2, '├': 0xc3, '─': 0xc4,
	'│': 0xb3, '┤': 0xb4,
	'┼': 0xc5, 'ã': 0xc6, 'Ã': 0xc7, '╚': 0xc8, '╔': 0xc9, '╩': 0xca, '╦': 0xcb, '╠': 0xcc,
	'═': 0xcd, '╬': 0xce, '¤': 0xcf, 'ð': 0xd0, 'Ð': 0xd1, 'Ê': 0xd2, 'Ë': 0xd3, 'È': 0xd4,
	'ı': 0xd5, 'Í': 0xd6, 'Î': 0xd7, 'Ï': 0xd8, '┘': 0xd9, '┌': 0xda, '█': 0xdb, '▄': 0xdc,
	'¦': 0xdd, 'Ì': 0xde, '▀': 0xdf, 'Ó': 0xe0, 'ß': 0xe1, 'Ô': 0xe2, 'Ò': 0xe3, 'õ': 0xe4,
	'Õ': 0xe5, 'µ': 0xe6, 'þ': 0xe7, 'Þ': 0xe8, 'Ú': 0xe9, 'Û': 0xea, 'Ù': 0xeb, 'ý': 0xec,
	'Ý': 0xed, '¯': 0xee, '´': 0xef, '­': 0xf0, '±': 0xf1, '‗': 0xf2, '¾': 0xf3, '¶': 0xf4,
	'§': 0xf5, '÷': 0xf6, '¸': 0xf7, '°': 0xf8, '¨': 0xf9, '·': 0xfa, '¹': 0xfb, '³': 0xfc,
	'²': 0xfd, '■': 0xfe,
}

// RawBytes anade bytes ya construidos. Lo usa el agente para reutilizar un
// raster cacheado sin volver a convertir la imagen.
func (b *Builder) RawBytes(data []byte) *Builder {
	b.buf.Write(data)
	return b
}
