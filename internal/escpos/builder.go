package escpos

import (
	"bytes"
	"image"
	"image/color"
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

	// SinAvance pide cortar sin avanzar nada. Es un valor aparte y no el 0
	// porque el 0 tiene que seguir significando "no indicado": asi el valor
	// por defecto de la estructura es el seguro y nadie corta texto por
	// olvidarse de rellenar el campo.
	SinAvance = -1

	// defaultLineHeight son los puntos de alto de una linea normal, el valor
	// de fabrica de la mayoria de termicas.
	defaultLineHeight = 30

	// TightLineSpacing aprieta el interlineado para no desperdiciar papel.
	TightLineSpacing = 24

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

// TextScale fija el tamano por multiplicador (GS ! n): 1 es el normal y 8 el
// maximo que admite ESC/POS, por separado en ancho y alto. FontSize sigue
// existiendo para los nombres de siempre.
func (b *Builder) TextScale(width, height int) *Builder {
	clamp := func(v int) int {
		if v < 1 {
			return 1
		}
		if v > 8 {
			return 8
		}
		return v
	}
	w, h := clamp(width), clamp(height)
	b.buf.Write([]byte{0x1d, 0x21, byte((w-1)<<4 | (h - 1))})
	return b
}

// Font selecciona la fuente interna (ESC M): "a" es la normal y "b" la
// condensada, que entra mas texto por linea.
func (b *Builder) Font(name string) *Builder {
	if strings.EqualFold(name, "b") || strings.EqualFold(name, "small") {
		b.buf.Write([]byte{0x1b, 0x4d, 0x01})
		return b
	}
	b.buf.Write([]byte{0x1b, 0x4d, 0x00})
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

// LineSpacing fija el alto de linea en puntos (ESC 3 n). Bajarlo aprieta el
// ticket; 0 restaura el valor de fabrica de la impresora (ESC 2).
func (b *Builder) LineSpacing(dots int) *Builder {
	if dots <= 0 {
		b.buf.Write([]byte{0x1b, 0x32})
		return b
	}
	if dots > 255 {
		dots = 255
	}
	b.buf.Write([]byte{0x1b, 0x33, byte(dots)})
	return b
}

// UpsideDown imprime el contenido girado 180 grados (ESC { n), de forma que
// el ticket sale con la cabecera en el extremo que queda abajo. Hay que
// activarlo al principio: afecta a lo que se imprima despues.
func (b *Builder) UpsideDown(on bool) *Builder {
	return b.onOff([]byte{0x1b, 0x7b}, on)
}

// Inverted imprime blanco sobre negro (GS B n). Util para destacar un total.
func (b *Builder) Inverted(on bool) *Builder {
	return b.onOff([]byte{0x1d, 0x42}, on)
}

// Rotate90 gira cada caracter 90 grados (ESC V n).
func (b *Builder) Rotate90(on bool) *Builder {
	return b.onOff([]byte{0x1b, 0x56}, on)
}

// LeftMargin fija el margen izquierdo en puntos (GS L).
func (b *Builder) LeftMargin(dots int) *Builder {
	if dots < 0 {
		dots = 0
	}
	b.buf.Write([]byte{0x1d, 0x4c, byte(dots % 256), byte(dots / 256)})
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

// CutMode indica como cortar el papel.
type CutMode string

const (
	CutFull    CutMode = "full"    // corte completo
	CutPartial CutMode = "partial" // deja un punto de union; lo normal en POS
	CutNone    CutMode = "none"    // no cortar
)

// Cut avanza y corta en un solo comando (GS V 66 n). Hacerlo con "avanzar y
// luego cortar" por separado deja al descubierto la distancia entre el
// cabezal y la cuchilla, que es justo lo que cortaba la ultima linea.
func (b *Builder) Cut(mode CutMode, feedLines int) *Builder {
	if mode == CutNone {
		return b
	}
	if feedLines < 0 {
		feedLines = 0
	}
	if feedLines > 255 {
		feedLines = 255
	}
	// GS V 66 n: avanza n puntos hasta la posicion de corte y hace corte
	// parcial. Se traduce de lineas a puntos con la altura de linea tipica.
	dots := feedLines * defaultLineHeight
	if dots > 255 {
		dots = 255
	}
	if mode == CutFull {
		// GS V 65 n hace lo mismo pero con corte completo.
		b.buf.Write([]byte{0x1d, 0x56, 65, byte(dots)})
		return b
	}
	b.buf.Write([]byte{0x1d, 0x56, 66, byte(dots)})
	return b
}

// CutLegacy usa GS V 0 sin avance, para impresoras que no entienden la
// funcion B del comando de corte.
func (b *Builder) CutLegacy() *Builder {
	b.buf.Write([]byte{0x1d, 0x56, 0x00})
	return b
}

func (b *Builder) DrawerKick() *Builder {
	b.buf.Write([]byte{0x1b, 0x70, 0x00, 0x19, 0xfa})
	return b
}

// QROptions controla el aspecto del codigo QR.
type QROptions struct {
	// ModuleSize es el lado de cada punto, de 1 a 16. Con 0 se calcula a
	// partir del ancho del papel y de cuanto texto lleve el QR.
	ModuleSize int
	// ECLevel es la correccion de errores: L, M, Q o H. Mas correccion
	// significa un QR mas grande pero legible aunque se manche. Por defecto M.
	ECLevel string
	// PaperWidth en puntos, para calcular ModuleSize cuando vale 0.
	PaperWidth int
}

// QR imprime un codigo QR con los valores por defecto.
func (b *Builder) QR(data string) *Builder {
	return b.QRWith(data, QROptions{})
}

// QRWith imprime un codigo QR con el tamano y la correccion indicados.
func (b *Builder) QRWith(data string, opt QROptions) *Builder {
	payload := []byte(data)
	if len(payload) == 0 || len(payload) > MaxQRLen {
		return b
	}
	module := opt.ModuleSize
	if module <= 0 {
		module = AutoQRModule(len(payload), opt.PaperWidth)
	}
	if module < 1 {
		module = 1
	}
	if module > 16 {
		module = 16
	}

	// GS ( k: modelo 2
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x04, 0x00, 0x31, 0x41, 0x32, 0x00})
	// tamano del modulo
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x43, byte(module)})
	// nivel de correccion
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x45, qrECByte(opt.ECLevel)})
	// datos
	pLen := len(payload) + 3
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, byte(pLen % 256), byte(pLen / 256), 0x31, 0x50, 0x30})
	b.buf.Write(payload)
	// imprimir
	b.buf.Write([]byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x51, 0x30})
	return b
}

func qrECByte(level string) byte {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "L":
		return 48
	case "Q":
		return 50
	case "H":
		return 51
	default: // M
		return 49
	}
}

// AutoQRModule elige el tamano de punto mas grande que deja el QR dentro del
// papel. Con el tamano fijo de antes, un QR con mucho texto se salia del
// ancho y la impresora lo recortaba.
func AutoQRModule(dataLen, paperWidth int) int {
	if paperWidth <= 0 {
		paperWidth = 384
	}
	// Lado del simbolo en modulos, aproximado por la capacidad en modo byte
	// del modelo 2 con correccion M.
	modules := 25
	switch {
	case dataLen > 1000:
		modules = 129
	case dataLen > 500:
		modules = 97
	case dataLen > 250:
		modules = 73
	case dataLen > 120:
		modules = 57
	case dataLen > 60:
		modules = 45
	case dataLen > 30:
		modules = 37
	}
	// Se deja un margen del 10% para la zona de silencio.
	size := (paperWidth * 9 / 10) / modules
	if size < 1 {
		size = 1
	}
	if size > 16 {
		size = 16
	}
	return size
}

type BarcodeType byte

const (
	BarcodeUPCA    BarcodeType = 65
	BarcodeUPCE    BarcodeType = 66
	BarcodeEAN13   BarcodeType = 67
	BarcodeEAN8    BarcodeType = 68
	BarcodeCode39  BarcodeType = 69
	BarcodeITF     BarcodeType = 70
	BarcodeCodabar BarcodeType = 71
	BarcodeCode93  BarcodeType = 72
	BarcodeCode128 BarcodeType = 73
	BarcodePDF417  BarcodeType = 0
)

// BarcodeTypeByName traduce el nombre que llega por la API.
func BarcodeTypeByName(name string) BarcodeType {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "upca", "upc-a":
		return BarcodeUPCA
	case "upce", "upc-e":
		return BarcodeUPCE
	case "ean13", "ean-13":
		return BarcodeEAN13
	case "ean8", "ean-8":
		return BarcodeEAN8
	case "code39", "code-39":
		return BarcodeCode39
	case "itf":
		return BarcodeITF
	case "codabar":
		return BarcodeCodabar
	case "code93", "code-93":
		return BarcodeCode93
	case "pdf417":
		return BarcodePDF417
	default:
		return BarcodeCode128
	}
}

// BarcodeOptions controla el aspecto del codigo de barras.
type BarcodeOptions struct {
	Type BarcodeType
	// HeightDots es el alto en puntos, de 1 a 255. Por defecto 100.
	HeightDots int
	// ModuleWidth es el grosor de la barra fina, de 2 a 6. Con 0 se calcula
	// para que el codigo quepa en el papel.
	ModuleWidth int
	// HRI es donde se imprime el texto legible: none, above, below o both.
	HRI string
	// PaperWidth en puntos, para calcular ModuleWidth cuando vale 0.
	PaperWidth int
}

func (b *Builder) Barcode(data string) *Builder {
	return b.BarcodeWith(data, BarcodeOptions{Type: BarcodeCode128})
}

func (b *Builder) BarcodeWithType(kind BarcodeType, data string) *Builder {
	return b.BarcodeWith(data, BarcodeOptions{Type: kind})
}

// BarcodeWith imprime un codigo de barras con el tamano indicado.
func (b *Builder) BarcodeWith(data string, opt BarcodeOptions) *Builder {
	if opt.Type == BarcodePDF417 {
		return b.PDF417(data)
	}
	payload := []byte(data)
	if opt.Type == BarcodeCode128 && len(payload) > 0 && payload[0] != '{' {
		payload = append([]byte("{B"), payload...)
	}
	// byte(len(payload)) daba la vuelta con 256 bytes o mas: la impresora
	// leia unos pocos y ejecutaba el resto de los datos como comandos.
	if len(payload) == 0 || len(payload) > MaxBarcodeLen {
		return b
	}

	height := opt.HeightDots
	if height <= 0 {
		height = 100
	}
	if height > 255 {
		height = 255
	}
	width := opt.ModuleWidth
	if width <= 0 {
		width = AutoBarcodeWidth(len(payload), opt.PaperWidth)
	}
	if width < 2 {
		width = 2
	}
	if width > 6 {
		width = 6
	}

	b.buf.Write([]byte{0x1d, 0x48, hriByte(opt.HRI)})
	b.buf.Write([]byte{0x1d, 0x68, byte(height)})
	b.buf.Write([]byte{0x1d, 0x77, byte(width)})
	b.buf.Write([]byte{0x1d, 0x6b, byte(opt.Type), byte(len(payload))})
	b.buf.Write(payload)
	return b
}

func hriByte(pos string) byte {
	switch strings.ToLower(strings.TrimSpace(pos)) {
	case "none", "":
		return 0
	case "above", "arriba":
		return 1
	case "both", "ambos":
		return 3
	default: // below
		return 2
	}
}

// AutoBarcodeWidth elige el grosor de barra mas grande que deja el codigo
// dentro del papel.
func AutoBarcodeWidth(dataLen, paperWidth int) int {
	if paperWidth <= 0 {
		paperWidth = 384
	}
	if dataLen <= 0 {
		return 2
	}
	// Code128 gasta unos 11 modulos por caracter, mas el inicio y el final.
	modules := dataLen*11 + 35
	w := (paperWidth * 9 / 10) / modules
	if w < 2 {
		w = 2
	}
	if w > 6 {
		w = 6
	}
	return w
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
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return b
	}
	// Camino rapido para el gris que produce PrepareImage: se leen los
	// bytes directamente en vez de una llamada a interfaz por pixel.
	gray, fast := img.(*image.Gray)

	widthBytes := (width + 7) / 8
	xL, xH := byte(widthBytes%256), byte(widthBytes/256)

	// Se emite una banda por cada rasterBandRows filas en vez de un unico
	// "GS v 0" con toda la imagen: las termicas con poco buffer cortaban o se
	// colgaban con logos altos.
	b.buf.Grow(widthBytes*height + (height/rasterBandRows+1)*8)
	for y0 := 0; y0 < height; y0 += rasterBandRows {
		rows := rasterBandRows
		if y0+rows > height {
			rows = height - y0
		}
		b.buf.Write([]byte{0x1d, 0x76, 0x30, 0x00, xL, xH, byte(rows % 256), byte(rows / 256)})
		for y := y0; y < y0+rows; y++ {
			var row []uint8
			if fast {
				off := (y+bounds.Min.Y-gray.Rect.Min.Y)*gray.Stride + (bounds.Min.X - gray.Rect.Min.X)
				row = gray.Pix[off : off+width]
			}
			// Camino rapido: ancho multiplo de 8 sobre un buffer de grises,
			// que es lo que produce siempre PrepareImage. Se empaquetan los
			// ocho bits sin comprobar limites en cada uno.
			if fast && width%8 == 0 {
				for xb := 0; xb < widthBytes; xb++ {
					p := row[xb*8 : xb*8+8]
					var packed byte
					if p[0] < 128 {
						packed |= 0x80
					}
					if p[1] < 128 {
						packed |= 0x40
					}
					if p[2] < 128 {
						packed |= 0x20
					}
					if p[3] < 128 {
						packed |= 0x10
					}
					if p[4] < 128 {
						packed |= 0x08
					}
					if p[5] < 128 {
						packed |= 0x04
					}
					if p[6] < 128 {
						packed |= 0x02
					}
					if p[7] < 128 {
						packed |= 0x01
					}
					b.buf.WriteByte(packed)
				}
				continue
			}
			for xb := 0; xb < widthBytes; xb++ {
				var packed byte
				for bit := 0; bit < 8; bit++ {
					x := xb*8 + bit
					if x >= width {
						break
					}
					dark := false
					if fast {
						dark = row[x] < 128
					} else {
						dark = isDark(img.At(bounds.Min.X+x, bounds.Min.Y+y))
					}
					if dark {
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

// luma es un buffer en escala de grises. Trabajar sobre bytes en vez de sobre
// la interfaz image.Image evita una llamada y una asignacion por pixel: el
// pipeline completo de un logo pasaba de 27 ms y 774.000 asignaciones.
type luma struct {
	pix  []uint8
	w, h int
}

// newLuma convierte cualquier imagen a gris, aplanando la transparencia sobre
// blanco (el papel). Tiene camino rapido para los formatos que produce el
// descodificador estandar.
func newLuma(src image.Image) *luma {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := &luma{pix: make([]uint8, w*h), w: w, h: h}

	switch im := src.(type) {
	case *image.Gray:
		for y := 0; y < h; y++ {
			off := (y+b.Min.Y-im.Rect.Min.Y)*im.Stride + (b.Min.X - im.Rect.Min.X)
			copy(out.pix[y*w:(y+1)*w], im.Pix[off:off+w])
		}
	case *image.RGBA:
		for y := 0; y < h; y++ {
			off := (y+b.Min.Y-im.Rect.Min.Y)*im.Stride + (b.Min.X-im.Rect.Min.X)*4
			row := im.Pix[off : off+w*4]
			for x := 0; x < w; x++ {
				p := row[x*4 : x*4+4]
				// RGBA viene con el alfa premultiplicado.
				out.pix[y*w+x] = flattenLuma(p[0], p[1], p[2], p[3], true)
			}
		}
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			off := (y+b.Min.Y-im.Rect.Min.Y)*im.Stride + (b.Min.X-im.Rect.Min.X)*4
			row := im.Pix[off : off+w*4]
			for x := 0; x < w; x++ {
				p := row[x*4 : x*4+4]
				out.pix[y*w+x] = flattenLuma(p[0], p[1], p[2], p[3], false)
			}
		}
	default:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
				out.pix[y*w+x] = flattenLuma(uint8(r>>8), uint8(g>>8), uint8(bl>>8), uint8(a>>8), true)
			}
		}
	}
	return out
}

// flattenLuma mezcla el pixel sobre blanco y devuelve su luminancia.
func flattenLuma(r, g, b, a uint8, premultiplied bool) uint8 {
	if a == 0xff {
		return uint8((299*uint32(r) + 587*uint32(g) + 114*uint32(b)) / 1000)
	}
	ar, ag, ab := uint32(r), uint32(g), uint32(b)
	if !premultiplied {
		ar = ar * uint32(a) / 255
		ag = ag * uint32(a) / 255
		ab = ab * uint32(a) / 255
	}
	inv := 255 - uint32(a)
	ar += inv
	ag += inv
	ab += inv
	l := (299*ar + 587*ag + 114*ab) / 1000
	if l > 255 {
		l = 255
	}
	return uint8(l)
}

// trim recorta las filas y columnas totalmente claras de los bordes, para que
// el relleno que traiga el PNG del logo no se imprima como papel en blanco.
func (l *luma) trim() {
	const blanco = 0xf0
	claro := func(i int) bool { return l.pix[i] >= blanco }

	top := 0
	for top < l.h {
		vacia := true
		for x := 0; x < l.w && vacia; x++ {
			vacia = claro(top*l.w + x)
		}
		if !vacia {
			break
		}
		top++
	}
	if top == l.h {
		return // imagen completamente en blanco: no se recorta nada
	}
	bottom := l.h
	for bottom > top {
		vacia := true
		for x := 0; x < l.w && vacia; x++ {
			vacia = claro((bottom-1)*l.w + x)
		}
		if !vacia {
			break
		}
		bottom--
	}
	left := 0
	for left < l.w {
		vacia := true
		for y := top; y < bottom && vacia; y++ {
			vacia = claro(y*l.w + left)
		}
		if !vacia {
			break
		}
		left++
	}
	right := l.w
	for right > left {
		vacia := true
		for y := top; y < bottom && vacia; y++ {
			vacia = claro(y*l.w + right - 1)
		}
		if !vacia {
			break
		}
		right--
	}
	if top == 0 && left == 0 && bottom == l.h && right == l.w {
		return
	}
	nw, nh := right-left, bottom-top
	pix := make([]uint8, nw*nh)
	for y := 0; y < nh; y++ {
		copy(pix[y*nw:(y+1)*nw], l.pix[(top+y)*l.w+left:(top+y)*l.w+right])
	}
	l.pix, l.w, l.h = pix, nw, nh
}

// scale reduce la imagen promediando cada bloque de origen. Antes se tomaba
// el pixel mas cercano, que en un logo con texto produce dientes de sierra.
//
// Los limites de cada columna se calculan una sola vez: hacerlo dentro del
// bucle suponia cuatro divisiones enteras por pixel, el 61% del tiempo de
// preparar una imagen.
func (l *luma) scale(dstW, dstH int) *luma {
	if dstW == l.w && dstH == l.h {
		return l
	}
	out := &luma{pix: make([]uint8, dstW*dstH), w: dstW, h: dstH}

	colStart := make([]int32, dstW)
	colEnd := make([]int32, dstW)
	for x := 0; x < dstW; x++ {
		s0 := x * l.w / dstW
		s1 := (x + 1) * l.w / dstW
		if s1 <= s0 {
			s1 = s0 + 1
		}
		if s1 > l.w {
			s1 = l.w
		}
		colStart[x], colEnd[x] = int32(s0), int32(s1)
	}

	for y := 0; y < dstH; y++ {
		sy0 := y * l.h / dstH
		sy1 := (y + 1) * l.h / dstH
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		if sy1 > l.h {
			sy1 = l.h
		}
		dst := out.pix[y*dstW : (y+1)*dstW]

		if sy1-sy0 == 1 {
			// Caso habitual al reducir poco: una sola fila de origen.
			src := l.pix[sy0*l.w : (sy0+1)*l.w]
			for x := 0; x < dstW; x++ {
				x0, x1 := colStart[x], colEnd[x]
				if x1-x0 == 1 {
					dst[x] = src[x0]
					continue
				}
				var sum uint32
				for _, v := range src[x0:x1] {
					sum += uint32(v)
				}
				dst[x] = uint8(sum / uint32(x1-x0))
			}
			continue
		}

		for x := 0; x < dstW; x++ {
			x0, x1 := colStart[x], colEnd[x]
			var sum uint32
			for sy := sy0; sy < sy1; sy++ {
				for _, v := range l.pix[sy*l.w+int(x0) : sy*l.w+int(x1)] {
					sum += uint32(v)
				}
			}
			dst[x] = uint8(sum / uint32(int32(sy1-sy0)*(x1-x0)))
		}
	}
	return out
}

// dither aplica Floyd-Steinberg y devuelve una imagen en blanco y negro puro.
func (l *luma) dither() *image.Gray {
	buf := make([]int32, l.w*l.h)
	for i, v := range l.pix {
		buf[i] = int32(v)
	}
	out := image.NewGray(image.Rect(0, 0, l.w, l.h))
	for y := 0; y < l.h; y++ {
		fila := y * l.w
		dst := out.Pix[y*out.Stride : y*out.Stride+l.w]
		for x := 0; x < l.w; x++ {
			i := fila + x
			old := buf[i]
			var nuevo int32 = 255
			if old < 128 {
				nuevo = 0
			}
			dst[x] = uint8(nuevo)
			err := old - nuevo
			if err == 0 {
				continue
			}
			if x+1 < l.w {
				buf[i+1] = clamp255(buf[i+1] + err*7/16)
			}
			if y+1 < l.h {
				if x > 0 {
					buf[i+l.w-1] = clamp255(buf[i+l.w-1] + err*3/16)
				}
				buf[i+l.w] = clamp255(buf[i+l.w] + err*5/16)
				if x+1 < l.w {
					buf[i+l.w+1] = clamp255(buf[i+l.w+1] + err/16)
				}
			}
		}
	}
	return out
}

func clamp255(v int32) int32 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// PrepareImage deja la imagen lista para la impresora: recortada, escalada al
// ancho del papel y difuminada a blanco y negro.
func PrepareImage(src image.Image, maxWidth int) image.Image {
	if maxWidth <= 0 {
		maxWidth = 384
	}
	l := newLuma(src)
	if l.w <= 0 || l.h <= 0 {
		return image.NewGray(image.Rect(0, 0, 1, 1))
	}
	l.trim()

	dstW, dstH := l.w, l.h
	if l.w > maxWidth {
		dstW = maxWidth
		dstH = int(math.Round(float64(l.h) * float64(maxWidth) / float64(l.w)))
	}
	// El ancho del raster se cuenta en bytes, asi que conviene multiplo de 8.
	dstW = (dstW / 8) * 8
	if dstW < 8 {
		dstW = 8
	}
	if dstH < 1 {
		dstH = 1
	}
	return l.scale(dstW, dstH).dither()
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
