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
		"a":      {'a'},
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
