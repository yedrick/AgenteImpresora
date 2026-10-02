package render

import (
	"strings"
	"testing"
)

const facturaHTML = `<!DOCTYPE html><html><head><style>body{font:12px}</style></head><body>
<h1>CAFETERIA CENTRAL</h1>
<p align="center">Av. Siempre Viva 742</p>
<hr>
<p>Cliente: <b>Ana Pe&ntilde;a</b></p>
<table>
<tr><td>Producto</td><td>Cant</td><td>Bs</td></tr>
<tr><td>Caf&eacute; con leche</td><td>2</td><td>24.00</td></tr>
<tr><td>Empanada de queso</td><td>3</td><td>28.50</td></tr>
<tr><td>Jugo de naranja</td><td>1</td><td>10.00</td></tr>
</table>
<hr>
<p align="right">TOTAL: <b>Bs 62.50</b></p>
<p align="center">Gracias por su compra</p>
</body></html>`

func BenchmarkRenderHTML(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := NewHTMLToESCPOS()
		h.SetColumns(48)
		_ = h.Render(facturaHTML)
	}
}

func BenchmarkRenderHTMLGrande(b *testing.B) {
	grande := strings.Replace(facturaHTML, "</table>",
		strings.Repeat("<tr><td>Articulo de relleno</td><td>1</td><td>1.00</td></tr>", 100)+"</table>", 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h := NewHTMLToESCPOS()
		h.SetColumns(48)
		_ = h.Render(grande)
	}
}
