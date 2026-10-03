package layout

import (
	"image"
	"math/rand"
	"strings"
	"testing"
)

// Propiedad que debe cumplirse SIEMPRE: componer un bloque o falla con un
// error, o devuelve una imagen del ancho exacto pedido y dentro del alto
// maximo. Nunca puede entrar en panico ni devolver algo deforme.
func TestPropiedadesConEntradasHostiles(t *testing.T) {
	r := rand.New(rand.NewSource(7))

	textos := []string{
		"", " ", "a", "Café con leche", strings.Repeat("X", 500),
		strings.Repeat("palabra ", 200), "\n\n\n", "\t\t", "日本語のテキスト",
		"a\nb\nc", strings.Repeat("ñ", 300), "<script>", "\x00\x01\x02",
	}
	tipos := []string{"", "text", "qr", "barcode", "image", "rule", "space", "desconocido"}
	tamanos := []string{"", "xs", "s", "m", "l", "xl", "xxl", "inventado"}
	alineaciones := []string{"", "left", "center", "right", "basura"}

	// Valores pensados para romper: negativos, cero, enormes.
	nums := []int{-1000, -1, 0, 1, 7, 100, 576, 10000, 1 << 20}

	elem := func() Item {
		return Item{
			Type:        tipos[r.Intn(len(tipos))],
			Text:        textos[r.Intn(len(textos))],
			Size:        tamanos[r.Intn(len(tamanos))],
			SizePx:      nums[r.Intn(len(nums))],
			Bold:        r.Intn(2) == 0,
			Mono:        r.Intn(2) == 0,
			Invert:      r.Intn(2) == 0,
			Align:       alineaciones[r.Intn(len(alineaciones))],
			QR:          textos[r.Intn(len(textos))],
			QRSize:      nums[r.Intn(len(nums))],
			QREC:        []string{"", "L", "M", "Q", "H", "zzz"}[r.Intn(6)],
			Barcode:     []string{"", "123", "ABC-123", strings.Repeat("9", 300), "café"}[r.Intn(5)],
			BarcodeType: []string{"", "code128", "inventado"}[r.Intn(3)],
			BarHeight:   nums[r.Intn(len(nums))],
			Image:       []string{"", "no-base64", "data:image/png;base64,QQ=="}[r.Intn(3)],
			Height:      nums[r.Intn(len(nums))],
			Gap:         nums[r.Intn(len(nums))],
		}
	}

	for caso := 0; caso < 400; caso++ {
		var rows []Row
		for f := 0; f < r.Intn(5); f++ {
			var cols []Col
			for c := 0; c < r.Intn(6); c++ {
				var items []Item
				for i := 0; i < r.Intn(4); i++ {
					items = append(items, elem())
				}
				cols = append(cols, Col{
					Weight: nums[r.Intn(len(nums))],
					Dots:   nums[r.Intn(len(nums))],
					Align:  alineaciones[r.Intn(len(alineaciones))],
					Pad:    nums[r.Intn(len(nums))],
					Border: r.Intn(2) == 0,
					Items:  items,
				})
			}
			fila := Row{Cols: cols, Align: alineaciones[r.Intn(len(alineaciones))],
				Border: r.Intn(2) == 0, MinHeight: nums[r.Intn(len(nums))]}
			if r.Intn(3) == 0 {
				g := nums[r.Intn(len(nums))]
				fila.Gap = &g
			}
			rows = append(rows, fila)
		}
		ancho := []int{0, -5, 1, 384, 512, 576, 99999}[r.Intn(7)]
		l := Layout{
			Width:   ancho,
			Gap:     nil,
			Padding: nums[r.Intn(len(nums))],
			Border:  r.Intn(2) == 0,
			Rows:    rows,
		}
		if r.Intn(3) == 0 {
			g := nums[r.Intn(len(nums))]
			l.Gap = &g
		}

		img, err := Render(l)
		if err != nil {
			continue // fallar con un error es una respuesta valida
		}
		g, ok := img.(*image.Gray)
		if !ok {
			t.Fatalf("caso %d: devolvio %T", caso, img)
		}
		esperado := l.Width
		if esperado <= 0 {
			esperado = 576
		}
		if got := g.Bounds().Dx(); got != esperado {
			t.Fatalf("caso %d: ancho %d, pedido %d\n%+v", caso, got, esperado, l)
		}
		if h := g.Bounds().Dy(); h < 1 || h > MaxHeight {
			t.Fatalf("caso %d: alto %d fuera de rango", caso, h)
		}
		// Todos los puntos deben ser blanco o negro puro: el empaquetado de
		// la impresora asume eso.
		for i, v := range g.Pix {
			if v != 0 && v != 255 {
				t.Fatalf("caso %d: el punto %d vale %d, deberia ser 0 o 255", caso, i, v)
			}
		}
	}
}
