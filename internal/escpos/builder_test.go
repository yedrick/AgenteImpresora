package escpos

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// byte(len(payload)) daba la vuelta a partir de 256 bytes: la impresora leia
// unos pocos y ejecutaba el resto de los datos como comandos ESC/POS.
func TestBarcodeNoDesbordaElByteDeLongitud(t *testing.T) {
	for _, n := range []int{1, 100, 253} {
		data := bytes.Repeat([]byte("A"), n)
		out := New().Barcode(string(data)).Bytes()
		idx := bytes.Index(out, []byte{0x1d, 0x6b, byte(BarcodeCode128)})
		if idx < 0 {
			t.Fatalf("len=%d: no se emitio el codigo de barras", n)
		}
		// Code128 antepone "{B"
		if got, want := int(out[idx+3]), n+2; got != want {
			t.Fatalf("len=%d: longitud declarada %d, real %d", n, got, want)
		}
	}
	for _, n := range []int{254, 300, 5000} {
		out := New().Barcode(string(bytes.Repeat([]byte("A"), n))).Bytes()
		if len(out) != 0 {
			t.Fatalf("len=%d: deberia descartarse en vez de emitir un comando corrupto, salieron %d bytes", n, len(out))
		}
	}
}

func TestQRRespetaSuLimite(t *testing.T) {
	if out := New().QR("https://ejemplo.com").Bytes(); len(out) == 0 {
		t.Fatal("un QR normal deberia emitirse")
	}
	if out := New().QR(string(bytes.Repeat([]byte("A"), MaxQRLen+1))).Bytes(); len(out) != 0 {
		t.Fatal("un QR por encima del limite deberia descartarse")
	}
	if out := New().QR("").Bytes(); len(out) != 0 {
		t.Fatal("un QR vacio deberia descartarse")
	}
}

// La cuchilla esta varias lineas por encima del cabezal: avanzar solo 1
// cortaba la ultima linea del ticket.
func TestElAvanceAntesDelCorteEsSuficiente(t *testing.T) {
	if CutFeedLines < 3 {
		t.Fatalf("CutFeedLines=%d es muy poco: se corta la ultima linea", CutFeedLines)
	}
}

func TestEncodeCP850(t *testing.T) {
	cases := map[string][]byte{
		"a": {'a'},
		"ñ": {0xa4},
		"é": {0x82},
		"ü": {0x81},
		"°": {0xf8},
		"│": {0xb3},
	}
	for in, want := range cases {
		if got := EncodeCP850(in); !bytes.Equal(got, want) {
			t.Fatalf("%q -> %#v, esperaba %#v", in, got, want)
		}
	}
	// Caracteres tipograficos que CP850 no tiene: antes salian como '?'.
	for in, want := range map[string]string{
		"•": "*", "…": "...", "—": "-", "€": "EUR",
	} {
		if got := string(EncodeCP850(in)); got != want {
			t.Fatalf("%q -> %q, esperaba %q", in, got, want)
		}
	}
	// Lo que no se puede representar de ninguna forma sigue siendo '?'.
	if got := string(EncodeCP850("中")); got != "?" {
		t.Fatalf("esperaba '?', obtuve %q", got)
	}
}

// Una imagen alta debe trocearse en bandas: muchas termicas desbordan el
// buffer con un unico GS v 0.
func TestLaImagenSeTroceaEnBandas(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.Black)
		}
	}
	out := New().Image(img).Bytes()
	bandas := bytes.Count(out, []byte{0x1d, 0x76, 0x30, 0x00})
	if bandas < 2 {
		t.Fatalf("esperaba varias bandas para 400 filas, encontre %d", bandas)
	}
	// Las filas declaradas en todas las bandas deben sumar la altura real.
	total := 0
	for i := 0; i+7 < len(out); i++ {
		if bytes.Equal(out[i:i+4], []byte{0x1d, 0x76, 0x30, 0x00}) {
			total += int(out[i+6]) + int(out[i+7])*256
		}
	}
	if total != 400 {
		t.Fatalf("las bandas suman %d filas, la imagen tiene 400", total)
	}
}

func rellenar(img *image.RGBA, c color.Color) *image.RGBA {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func densidad(g *image.Gray) float64 {
	var negros int
	b := g.Bounds()
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			if g.Pix[y*g.Stride+x] < 128 {
				negros++
			}
		}
	}
	return float64(negros) / float64(b.Dx()*b.Dy())
}

func TestPrepareImageDensidad(t *testing.T) {
	cases := []struct {
		nombre   string
		c        color.Color
		min, max float64
	}{
		{"negro", color.RGBA{0, 0, 0, 255}, 0.99, 1.0},
		{"blanco", color.RGBA{255, 255, 255, 255}, 0.0, 0.01},
		{"gris medio", color.RGBA{128, 128, 128, 255}, 0.35, 0.65},
		{"transparente", color.RGBA{0, 0, 0, 0}, 0.0, 0.01}, // se aplana sobre papel blanco
	}
	for _, c := range cases {
		img := rellenar(image.NewRGBA(image.Rect(0, 0, 64, 64)), c.c)
		g, ok := PrepareImage(img, 64).(*image.Gray)
		if !ok {
			t.Fatalf("%s: PrepareImage deberia devolver *image.Gray", c.nombre)
		}
		if d := densidad(g); d < c.min || d > c.max {
			t.Fatalf("%s: densidad de tinta %.3f, esperaba entre %.2f y %.2f", c.nombre, d, c.min, c.max)
		}
	}
}

// Un blanco puro solo debe producir bytes a cero, y un negro puro, a 0xFF.
func TestRasterDeColoresPlanos(t *testing.T) {
	negro := PrepareImage(rellenar(image.NewRGBA(image.Rect(0, 0, 32, 8)), color.Black), 32)
	out := New().Image(negro).Bytes()
	datos := out[8:] // tras la cabecera GS v 0
	for i, b := range datos {
		if b != 0xff {
			t.Fatalf("byte %d del raster negro es %#x, esperaba 0xff", i, b)
		}
	}

	blanco := PrepareImage(rellenar(image.NewRGBA(image.Rect(0, 0, 32, 8)), color.White), 32)
	out = New().Image(blanco).Bytes()
	for i, b := range out[8:] {
		if b != 0x00 {
			t.Fatalf("byte %d del raster blanco es %#x, esperaba 0x00", i, b)
		}
	}
}

func TestPrepareImageRespetaLaProporcion(t *testing.T) {
	img := rellenar(image.NewRGBA(image.Rect(0, 0, 800, 400)), color.Black)
	g := PrepareImage(img, 576).(*image.Gray)
	w, h := g.Bounds().Dx(), g.Bounds().Dy()
	if w != 576 {
		t.Fatalf("ancho %d, esperaba 576", w)
	}
	// 800x400 escalado a 576 deberia dar unos 288 de alto.
	if h < 280 || h > 296 {
		t.Fatalf("alto %d, esperaba ~288 (proporcion no respetada)", h)
	}
	if w%8 != 0 {
		t.Fatalf("el ancho %d deberia ser multiplo de 8", w)
	}
}

// Una imagen mas estrecha que el papel no se amplia: ampliarla solo
// emborronaria el logo.
func TestPrepareImageNoAmplia(t *testing.T) {
	img := rellenar(image.NewRGBA(image.Rect(0, 0, 100, 50)), color.Black)
	g := PrepareImage(img, 576).(*image.Gray)
	if w := g.Bounds().Dx(); w > 100 {
		t.Fatalf("ancho %d: no deberia ampliarse por encima de 100", w)
	}
}

// El relleno blanco del borde se recorta para no imprimir papel vacio.
func TestPrepareImageRecortaElBorde(t *testing.T) {
	img := rellenar(image.NewRGBA(image.Rect(0, 0, 200, 200)), color.White)
	for y := 80; y < 120; y++ {
		for x := 80; x < 120; x++ {
			img.Set(x, y, color.Black)
		}
	}
	g := PrepareImage(img, 200).(*image.Gray)
	if w, h := g.Bounds().Dx(), g.Bounds().Dy(); w > 56 || h > 56 {
		t.Fatalf("raster %dx%d: el borde blanco no se recorto (el contenido son 40x40)", w, h)
	}
}
