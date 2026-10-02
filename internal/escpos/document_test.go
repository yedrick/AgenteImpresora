package escpos

import (
	"bytes"
	"testing"
)

func TestDocumentoNoGastaPapelArriba(t *testing.T) {
	// Sin FeedTop no debe salir ni un salto de linea antes del contenido:
	// cada linea en blanco es papel tirado en cada ticket.
	b := Begin(DocOptions{PaperWidth: 384})
	if bytes.Contains(b.Bytes(), []byte{'\n'}) {
		t.Fatalf("el arranque mete saltos de linea: %q", b.Bytes())
	}
	if n := bytes.Count(b.Bytes(), []byte{0x1b, 0x64}); n != 0 {
		t.Fatalf("el arranque emite %d avances de papel, esperaba 0", n)
	}
}

func TestInterlineadoApretado(t *testing.T) {
	b := Begin(DocOptions{PaperWidth: 384, LineSpacing: TightLineSpacing})
	if !bytes.Contains(b.Bytes(), []byte{0x1b, 0x33, TightLineSpacing}) {
		t.Fatalf("falta ESC 3 n: %q", b.Bytes())
	}
	// Al cerrar se devuelve el valor de fabrica, para no dejar la impresora
	// configurada para el siguiente trabajo.
	b.End(DocOptions{LineSpacing: TightLineSpacing, Cut: CutPartial})
	if !bytes.Contains(b.Bytes(), []byte{0x1b, 0x32}) {
		t.Fatal("al cerrar deberia restaurarse el interlineado de fabrica")
	}
}

func TestImpresionAlReves(t *testing.T) {
	b := Begin(DocOptions{PaperWidth: 384, UpsideDown: true})
	if !bytes.Contains(b.Bytes(), []byte{0x1b, 0x7b, 1}) {
		t.Fatalf("falta ESC { 1 al arrancar: %q", b.Bytes())
	}
	antes := len(b.Bytes())
	b.End(DocOptions{UpsideDown: true, Cut: CutPartial})
	if !bytes.Contains(b.Bytes()[antes:], []byte{0x1b, 0x7b, 0}) {
		t.Fatal("al cerrar deberia desactivarse el giro")
	}
}

func TestModosDeCorte(t *testing.T) {
	cases := map[CutMode][]byte{
		CutPartial: {0x1d, 0x56, 66},
		CutFull:    {0x1d, 0x56, 65},
	}
	for mode, want := range cases {
		b := Begin(DocOptions{PaperWidth: 384})
		b.End(DocOptions{Cut: mode, FeedBottom: CutFeedLines})
		if !bytes.Contains(b.Bytes(), want) {
			t.Fatalf("corte %q: falta %v en %q", mode, want, b.Bytes())
		}
	}
	// Sin corte no debe emitirse ningun GS V.
	b := Begin(DocOptions{PaperWidth: 384})
	b.End(DocOptions{Cut: CutNone})
	if bytes.Contains(b.Bytes(), []byte{0x1d, 0x56}) {
		t.Fatal("con cut=none no deberia cortarse")
	}
}

// Avanzar menos de lo que separa el cabezal de la cuchilla corta la ultima
// linea del ticket.
func TestElAvanceDeCorteTieneMinimo(t *testing.T) {
	b := Begin(DocOptions{PaperWidth: 384})
	b.End(DocOptions{Cut: CutPartial, FeedBottom: 1})
	out := b.Bytes()
	i := bytes.Index(out, []byte{0x1d, 0x56, 66})
	if i < 0 {
		t.Fatal("no se emitio el corte")
	}
	if got, min := int(out[i+3]), CutFeedLines*defaultLineHeight; got < min {
		t.Fatalf("avance de %d puntos, el minimo es %d", got, min)
	}
}

func TestColumnasPorAnchoDePapel(t *testing.T) {
	for ancho, cols := range map[int]int{384: 32, 512: 42, 576: 48, 0: 32} {
		if got := (DocOptions{PaperWidth: ancho}).Columns(); got != cols {
			t.Fatalf("ancho %d -> %d columnas, esperaba %d", ancho, got, cols)
		}
	}
}

func TestEscalaDeTexto(t *testing.T) {
	cases := []struct {
		w, h int
		want byte
	}{
		{1, 1, 0x00}, {2, 2, 0x11}, {3, 1, 0x20}, {1, 4, 0x03}, {8, 8, 0x77},
		{0, 0, 0x00},   // se acota al minimo
		{99, 99, 0x77}, // y al maximo
	}
	for _, c := range cases {
		b := New().TextScale(c.w, c.h)
		want := []byte{0x1d, 0x21, c.want}
		if !bytes.Equal(b.Bytes(), want) {
			t.Fatalf("TextScale(%d,%d) = %v, esperaba %v", c.w, c.h, b.Bytes(), want)
		}
	}
}

// El tamano del QR debe adaptarse al papel y a cuanto texto lleve.
func TestTamanoAutomaticoDelQR(t *testing.T) {
	corto58 := AutoQRModule(20, 384)
	corto80 := AutoQRModule(20, 576)
	largo58 := AutoQRModule(800, 384)
	if corto80 <= corto58 {
		t.Fatalf("con papel mas ancho el QR deberia poder ser mayor: 58mm=%d 80mm=%d", corto58, corto80)
	}
	if largo58 >= corto58 {
		t.Fatalf("con mas datos el punto deberia ser menor: corto=%d largo=%d", corto58, largo58)
	}
	for _, v := range []int{corto58, corto80, largo58} {
		if v < 1 || v > 16 {
			t.Fatalf("tamano de modulo fuera de rango: %d", v)
		}
	}
}

func TestTamanoAutomaticoDelCodigoDeBarras(t *testing.T) {
	if AutoBarcodeWidth(10, 576) < AutoBarcodeWidth(40, 576) {
		t.Fatal("un codigo mas largo deberia usar barras mas finas")
	}
	for _, w := range []int{AutoBarcodeWidth(10, 384), AutoBarcodeWidth(100, 384)} {
		if w < 2 || w > 6 {
			t.Fatalf("grosor fuera de rango: %d", w)
		}
	}
}

func TestNivelDeCorreccionDelQR(t *testing.T) {
	for level, want := range map[string]byte{"L": 48, "M": 49, "Q": 50, "H": 51, "": 49, "x": 49} {
		b := New().QRWith("hola", QROptions{ECLevel: level, ModuleSize: 4})
		if !bytes.Contains(b.Bytes(), []byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x45, want}) {
			t.Fatalf("nivel %q: esperaba el byte %d", level, want)
		}
	}
}
