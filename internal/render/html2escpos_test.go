package render

import (
	"bytes"
	"strings"
	"testing"
)

func renderHTML(t *testing.T, markup string, cols int) []byte {
	t.Helper()
	h := NewHTMLToESCPOS()
	h.SetColumns(cols)
	return h.RenderNoCut(markup)
}

// Antes, la rama de comentario del tokenizador capturaba <!DOCTYPE> y saltaba
// hasta el siguiente "-->", que no existe: el documento entero se descartaba y
// salia papel en blanco.
func TestDoctypeNoDescartaElDocumento(t *testing.T) {
	out := renderHTML(t, `<!DOCTYPE html><html><body><p>Total 120</p></body></html>`, 32)
	if !bytes.Contains(out, []byte("Total 120")) {
		t.Fatalf("el contenido se perdio con <!DOCTYPE>: %q", out)
	}
}

func TestComentarioSiSeDescarta(t *testing.T) {
	out := renderHTML(t, `<p>antes</p><!-- oculto --><p>despues</p>`, 32)
	if bytes.Contains(out, []byte("oculto")) {
		t.Fatalf("se imprimio el comentario: %q", out)
	}
	for _, want := range []string{"antes", "despues"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("falta %q en %q", want, out)
		}
	}
}

// El contenido de <style>, <script> y <title> no es texto para imprimir.
func TestStyleYScriptNoSeImprimen(t *testing.T) {
	out := renderHTML(t, `<html><head><title>pestana</title><style>body{color:red}</style></head><body><script>var x=1</script><p>Hola</p></body></html>`, 32)
	for _, bad := range []string{"color:red", "var x", "pestana"} {
		if bytes.Contains(out, []byte(bad)) {
			t.Fatalf("se imprimio %q: %q", bad, out)
		}
	}
	if !bytes.Contains(out, []byte("Hola")) {
		t.Fatalf("falta el cuerpo: %q", out)
	}
}

// Una etiqueta inline no debe partir la linea.
func TestEtiquetaInlineNoParteLaLinea(t *testing.T) {
	out := renderHTML(t, "<p>Total: <b>Bs 12.00</b></p>", 32)
	out = stripCommands(out)
	idx := bytes.Index(out, []byte("Total: "))
	if idx < 0 {
		t.Fatalf("falta el texto: %q", out)
	}
	resto := out[idx:]
	salto := bytes.IndexByte(resto, '\n')
	if salto < 0 || !bytes.Contains(resto[:salto], []byte("Bs 12.00")) {
		t.Fatalf("la etiqueta inline partio la linea: %q", resto)
	}
}

func TestHTMLIndentadoNoParteLaLinea(t *testing.T) {
	out := renderHTML(t, "<!DOCTYPE html>\n<html>\n  <body>\n    <p>Cliente:\n      <b>Ana</b>\n    </p>\n  </body>\n</html>", 32)
	// Se quitan los comandos ESC/POS: "Ana" va en negrita, asi que entre
	// medias hay secuencias de estilo aunque sea la misma linea.
	if !bytes.Contains(stripCommands(out), []byte("Cliente: Ana")) {
		t.Fatalf("los saltos del fuente partieron la linea: %q", out)
	}
}

// Las entidades se decodificaban iterando un map, con orden aleatorio.
func TestDecodificacionDeEntidadesEsDeterminista(t *testing.T) {
	first := renderHTML(t, `<p>&amp;lt; &eacute; &ntilde;</p>`, 32)
	for i := 0; i < 50; i++ {
		if got := renderHTML(t, `<p>&amp;lt; &eacute; &ntilde;</p>`, 32); !bytes.Equal(got, first) {
			t.Fatalf("salida no determinista:\n%q\n%q", first, got)
		}
	}
}

// Los acentos deben salir como su byte CP850, no como '?'.
func TestAcentosEnCP850(t *testing.T) {
	out := renderHTML(t, `<p>&uacute;&ntilde;&uuml;&deg;</p>`, 32)
	for _, want := range []byte{0xa3 /* u */, 0xa4 /* n */, 0x81 /* u */, 0xf8 /* grados */} {
		if !bytes.Contains(out, []byte{want}) {
			t.Fatalf("falta el byte CP850 %#x en %q", want, out)
		}
	}
	if bytes.Contains(out, []byte("?")) {
		t.Fatalf("algun caracter se degrado a '?': %q", out)
	}
}

// El marco de la tabla recortaba un byte de un caracter de caja de 3 bytes,
// dejando UTF-8 invalido que acababa impreso como '?'.
func TestTablaConBordeNoProduceInterrogantes(t *testing.T) {
	out := renderHTML(t, `<table><tr><td>Pan</td><td>10.00</td></tr><tr><td>Cafe</td><td>20.50</td></tr></table>`, 32)
	if bytes.Contains(out, []byte("?")) {
		t.Fatalf("el marco de la tabla produjo '?': %q", out)
	}
	if !bytes.Contains(out, []byte{0xda} /* esquina superior izquierda CP850 */) {
		t.Fatalf("falta el marco de la tabla: %q", out)
	}
}

// Ninguna linea impresa puede pasarse del ancho del papel.
func TestLasLineasRespetanElAncho(t *testing.T) {
	for _, cols := range []int{32, 42, 48} {
		out := renderHTML(t, `<table><tr><td>Producto muy largo de verdad</td><td>Cantidad</td><td>Precio</td></tr><tr><td>a</td><td>b</td><td>c</td></tr></table>`, cols)
		for _, line := range strings.Split(string(stripCommands(out)), "\n") {
			if n := len(line); n > cols {
				t.Fatalf("cols=%d: linea de %d caracteres: %q", cols, n, line)
			}
		}
	}
}

// stripCommands quita los comandos ESC/POS y deja solo lo que se imprime.
func stripCommands(b []byte) []byte {
	// Comandos de tres bytes: ESC X n / GS X n.
	esc3 := map[byte]bool{'a': true, 'E': true, '-': true, 'M': true, 't': true, 'd': true, '{': true, 'V': true, '3': true}
	gs3 := map[byte]bool{'!': true, 'B': true, 'h': true, 'w': true, 'H': true}
	var out []byte
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == 0x1b && i+1 < len(b) && (b[i+1] == '@' || b[i+1] == '2'):
			i++
		case b[i] == 0x1b && i+2 < len(b) && esc3[b[i+1]]:
			i += 2
		case b[i] == 0x1d && i+3 < len(b) && b[i+1] == 'V':
			i += 3 // GS V m n
		case b[i] == 0x1d && i+3 < len(b) && b[i+1] == 'L':
			i += 3 // GS L nL nH
		case b[i] == 0x1d && i+2 < len(b) && gs3[b[i+1]]:
			i += 2
		default:
			out = append(out, b[i])
		}
	}
	return out
}
