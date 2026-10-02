package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func body(t *testing.T, markup string, cols int) []byte {
	t.Helper()
	return NewHTMLToESCPOS().WithColumns(cols).WithPaperWidth(cols * 12).Body(markup)
}

func texto(b []byte) string { return string(stripCommands(b)) }

// El atributo style se partia por espacios, asi que "text-align: center" se
// rompia en pedazos y no se aplicaba.
func TestEstiloConEspaciosYComillas(t *testing.T) {
	out := body(t, `<p style="text-align: center; font-weight: bold">Centrado</p>`, 32)
	if !bytes.Contains(out, []byte{0x1b, 0x61, 1}) {
		t.Fatalf("no se centro: %q", out)
	}
	if !bytes.Contains(out, []byte{0x1b, 0x45, 1}) {
		t.Fatalf("no se puso en negrita: %q", out)
	}
}

func TestAtributosConComillas(t *testing.T) {
	a := parseAttrs(`align='center' width = "24" style="font-size: large" nowrap`)
	if a.get("align") != "center" {
		t.Fatalf("align: %q", a.get("align"))
	}
	if a.int("width") != 24 {
		t.Fatalf("width: %d", a.int("width"))
	}
	if a.style()["font-size"] != "large" {
		t.Fatalf("font-size: %q", a.style()["font-size"])
	}
	if _, ok := a["nowrap"]; !ok {
		t.Fatal("falta el atributo sin valor")
	}
}

func TestListas(t *testing.T) {
	out := texto(body(t, `<ul><li>Cafe</li><li>Pan</li></ul>`, 32))
	for _, want := range []string{"Cafe", "Pan"} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en %q", want, out)
		}
	}
	// CP850 no tiene el caracter de vineta, asi que sale como '*'.
	if !strings.Contains(out, "* Cafe") {
		t.Fatalf("las vinetas no aparecen: %q", out)
	}

	num := texto(body(t, `<ol><li>Uno</li><li>Dos</li><li>Tres</li></ol>`, 32))
	for _, want := range []string{"1. Uno", "2. Dos", "3. Tres"} {
		if !strings.Contains(num, want) {
			t.Fatalf("falta %q en %q", want, num)
		}
	}
}

func TestEncabezadosEscalan(t *testing.T) {
	h1 := body(t, `<h1>Titulo</h1>`, 32)
	h4 := body(t, `<h4>Titulo</h4>`, 32)
	if !bytes.Contains(h1, []byte{0x1d, 0x21, 0x11}) {
		t.Fatalf("h1 deberia ir a doble tamano: %q", h1)
	}
	if bytes.Contains(h4, []byte{0x1d, 0x21, 0x11}) {
		t.Fatalf("h4 no deberia ir a doble tamano: %q", h4)
	}
}

func TestTamanoDeFuentePorCSS(t *testing.T) {
	out := body(t, `<p><span style="font-size:x-large">GRANDE</span></p>`, 32)
	if !bytes.Contains(out, []byte{0x1d, 0x21, 0x22}) { // escala 3x3
		t.Fatalf("no se aplico x-large: %q", out)
	}
}

func TestImagenEnHTML(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.Black)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	b64 := base64Encode(buf.Bytes())

	out := body(t, `<p>antes</p><img src="data:image/png;base64,`+b64+`"><p>despues</p>`, 32)
	if !bytes.Contains(out, []byte{0x1d, 0x76, 0x30, 0x00}) {
		t.Fatal("no se emitio el raster de la imagen")
	}
	txt := texto(out)
	if !strings.Contains(txt, "antes") || !strings.Contains(txt, "despues") {
		t.Fatalf("se perdio el texto alrededor de la imagen: %q", txt)
	}
}

// Una imagen rota no debe tumbar el ticket entero.
func TestImagenInvalidaSeIgnora(t *testing.T) {
	out := texto(body(t, `<p>antes</p><img src="data:image/png;base64,NO-ES-BASE64"><p>despues</p>`, 32))
	if !strings.Contains(out, "antes") || !strings.Contains(out, "despues") {
		t.Fatalf("una imagen invalida rompio el resto: %q", out)
	}
}

func TestEtiquetasQRyBarcode(t *testing.T) {
	out := body(t, `<qr data="https://kollatek.com" size="5" ec="H"></qr>`, 32)
	if !bytes.Contains(out, []byte{0x1d, 0x28, 0x6b}) {
		t.Fatal("no se emitio el QR")
	}
	if !bytes.Contains(out, []byte{0x1d, 0x28, 0x6b, 0x03, 0x00, 0x31, 0x43, 5}) {
		t.Fatalf("no se aplico size=5: %q", out)
	}

	bc := body(t, `<barcode data="123456789" type="code128" height="80"></barcode>`, 32)
	if !bytes.Contains(bc, []byte{0x1d, 0x68, 80}) {
		t.Fatalf("no se aplico height=80: %q", bc)
	}
}

func TestPreConservaLosEspacios(t *testing.T) {
	out := texto(body(t, "<pre>a   b\nc   d</pre>", 32))
	if !strings.Contains(out, "a   b") {
		t.Fatalf("pre no conservo los espacios: %q", out)
	}
	if !strings.Contains(out, "c   d") {
		t.Fatalf("pre no conservo el salto de linea: %q", out)
	}
}

// En un ticket la ultima columna suele ser el importe y va a la derecha.
func TestLaUltimaColumnaVaALaDerecha(t *testing.T) {
	out := texto(body(t, `<table><tr><td>Cafe</td><td>12.00</td></tr></table>`, 32))
	// Se busca la linea de datos, no las del marco.
	var datos string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "Cafe") {
			datos = l
			break
		}
	}
	if datos == "" {
		t.Fatalf("no se encontro la fila: %q", out)
	}
	// El importe va pegado a la derecha, justo antes del borde si lo hay.
	limpia := strings.TrimRight(strings.TrimRight(datos, "\xb3"), " ")
	if !strings.HasSuffix(limpia, "12.00") {
		t.Fatalf("el importe no quedo a la derecha: %q", datos)
	}
}

func TestEncabezadoDeTablaEnNegrita(t *testing.T) {
	out := body(t, `<table><tr><th>Producto</th><th>Bs</th></tr><tr><td>Cafe</td><td>12</td></tr></table>`, 32)
	if !bytes.Contains(out, []byte{0x1b, 0x45, 1}) {
		t.Fatalf("el th deberia ir en negrita: %q", out)
	}
}

// Ninguna linea puede pasarse del ancho del papel, con o sin marco.
func TestTablasRespetanElAncho(t *testing.T) {
	htmls := []string{
		`<table><tr><td>Producto con nombre larguisimo de verdad</td><td>1234.56</td></tr></table>`,
		`<table><tr><td>a</td><td>b</td><td>c</td><td>d</td><td>e</td><td>f</td></tr></table>`,
		`<table><tr><td width="20">Fijo</td><td>Resto</td></tr></table>`,
	}
	for _, cols := range []int{32, 42, 48} {
		for _, markup := range htmls {
			for _, linea := range strings.Split(texto(body(t, markup, cols)), "\n") {
				if n := len(linea); n > cols {
					t.Fatalf("cols=%d: linea de %d caracteres: %q", cols, n, linea)
				}
			}
		}
	}
}

func base64Encode(b []byte) string {
	const tabla = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var sb strings.Builder
	for i := 0; i < len(b); i += 3 {
		var n uint32
		rem := len(b) - i
		n = uint32(b[i]) << 16
		if rem > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if rem > 2 {
			n |= uint32(b[i+2])
		}
		sb.WriteByte(tabla[(n>>18)&63])
		sb.WriteByte(tabla[(n>>12)&63])
		if rem > 1 {
			sb.WriteByte(tabla[(n>>6)&63])
		} else {
			sb.WriteByte('=')
		}
		if rem > 2 {
			sb.WriteByte(tabla[n&63])
		} else {
			sb.WriteByte('=')
		}
	}
	return sb.String()
}
