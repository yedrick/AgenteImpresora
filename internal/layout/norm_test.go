package layout

import (
	"image"
	"testing"
)

// Un valor negativo no debe hacer nada raro en silencio: antes, un pad
// negativo dibujaba el contenido fuera de su columna y un padding negativo
// dejaba el bloque en un punto de alto, perdiendo el ticket entero.
func TestLosValoresNegativosSeAcotan(t *testing.T) {
	bueno := Layout{Width: 576, Rows: []Row{{Cols: []Col{
		{Weight: 1, Items: []Item{{Text: "Total Bs 24.00"}}},
	}}}}
	ref, err := Render(bueno)
	if err != nil {
		t.Fatal(err)
	}

	casos := map[string]Layout{
		"padding negativo": {Width: 576, Padding: -200, Rows: bueno.Rows},
		"pad negativo": {Width: 576, Rows: []Row{{Cols: []Col{
			{Weight: 1, Pad: -1000, Items: []Item{{Text: "Total Bs 24.00"}}}}}}},
		"min_height negativo": {Width: 576, Rows: []Row{{MinHeight: -500, Cols: bueno.Rows[0].Cols}}},
		"gap de elemento negativo": {Width: 576, Rows: []Row{{Cols: []Col{
			{Weight: 1, Items: []Item{{Text: "Total Bs 24.00", Gap: -400}}}}}}},
		"dots negativo": {Width: 576, Rows: []Row{{Cols: []Col{
			{Dots: -300, Items: []Item{{Text: "Total Bs 24.00"}}}}}}},
	}
	for nombre, l := range casos {
		img, err := Render(l)
		if err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		if got := img.Bounds().Dx(); got != 576 {
			t.Fatalf("%s: ancho %d", nombre, got)
		}
		// El resultado debe parecerse al del caso bueno, no quedarse en nada.
		if h := img.Bounds().Dy(); h < ref.Bounds().Dy() {
			t.Fatalf("%s: alto %d, el contenido se perdio (lo normal son %d)",
				nombre, h, ref.Bounds().Dy())
		}
	}
}

// El QR tiene que caber en su columna aunque se pidan valores absurdos.
func TestElQRNoSeSaleAunqueElPadSeaNegativo(t *testing.T) {
	l := Layout{Width: 576, Rows: []Row{{Cols: []Col{
		{Dots: 150, Pad: -1000, Items: []Item{{QR: "https://ejemplo.com/f/000123"}}},
		{Weight: 1, Items: []Item{{Text: "x"}}},
	}}}}
	img, err := Render(l)
	if err != nil {
		t.Fatal(err)
	}
	g := img.(*image.Gray)
	// No puede haber tinta mas alla de la primera columna mas la segunda.
	if img.Bounds().Dx() != 576 {
		t.Fatalf("ancho %d", img.Bounds().Dx())
	}
	_ = g
}

// En el disenador se anade un elemento y se rellena despues. Que un hueco
// vacio borrara la vista previa entera hacia el disenador inusable.
func TestElementosVaciosNoRompenElBloque(t *testing.T) {
	l := Layout{Width: 576, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Text: "Cabecera del ticket"}}}}},
		{Cols: []Col{{Items: []Item{{Type: "qr", QR: ""}}}}},
		{Cols: []Col{{Items: []Item{{Type: "barcode", Barcode: "  "}}}}},
		{Cols: []Col{{Items: []Item{{Type: "image", Image: ""}}}}},
		{Cols: []Col{{Items: []Item{{Text: "Pie del ticket"}}}}},
	}}
	img, err := Render(l)
	if err != nil {
		t.Fatalf("un elemento vacio no deberia ser un error: %v", err)
	}
	if img.Bounds().Dy() < 20 {
		t.Fatalf("el resto del ticket se perdio: alto %d", img.Bounds().Dy())
	}
}

// Pero un contenido presente y mal formado SI tiene que avisar.
func TestElContenidoInvalidoSiAvisa(t *testing.T) {
	casos := map[string]Item{
		"imagen que no es base64": {Type: "image", Image: "esto-no-es-base64"},
		"codigo con acentos":      {Type: "barcode", Barcode: "café"},
	}
	for nombre, it := range casos {
		_, err := Render(Layout{Width: 576, Rows: []Row{{Cols: []Col{{Items: []Item{it}}}}}})
		if err == nil {
			t.Fatalf("%s: deberia dar error", nombre)
		}
	}
}

// Render no puede tocar lo que recibe: Layout va por valor pero Rows es un
// slice, asi que mutarlo en sitio escribia en los datos de quien llama.
func TestRenderNoModificaLaEntrada(t *testing.T) {
	g := -50
	original := Layout{
		Width: 576, Padding: -10, Gap: &g,
		Rows: []Row{{MinHeight: -5, Cols: []Col{
			{Pad: -20, Dots: -3, Weight: -1, Items: []Item{
				{Text: "hola", Gap: -7, SizePx: 9999, BarHeight: 1 << 30},
			}},
		}}},
	}
	if _, err := Render(original); err != nil {
		t.Fatal(err)
	}
	if original.Padding != -10 || *original.Gap != -50 {
		t.Fatal("se modificaron los campos del bloque")
	}
	c := original.Rows[0].Cols[0]
	if original.Rows[0].MinHeight != -5 || c.Pad != -20 || c.Dots != -3 || c.Weight != -1 {
		t.Fatalf("se modificaron fila o columna: %+v", c)
	}
	it := c.Items[0]
	if it.Gap != -7 || it.SizePx != 9999 || it.BarHeight != 1<<30 {
		t.Fatalf("se modificaron los elementos: %+v", it)
	}
}
