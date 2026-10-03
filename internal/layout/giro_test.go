package layout

import (
	"image"
	"testing"
)

// TestGirar180PoneLaTintaAlOtroLado: el comando ESC { de la impresora pone
// el modo "al reves", pero solo afecta a los caracteres. Una imagen raster
// (GS v 0) sale igual de derecha, y un bloque maquetado ES una imagen: por
// eso pedir giro no hacia nada y el ticket salia normal. Hay que girar los
// puntos antes de mandarlos.
func TestGirar180PoneLaTintaAlOtroLado(t *testing.T) {
	// Un bloque con el texto arriba y hueco debajo.
	img, err := Render(Layout{
		Width: 576, Padding: 0,
		Rows: []Row{
			{Cols: []Col{{Weight: 1, Items: []Item{{Text: "ARRIBA", Size: "xl"}}}}},
			{Cols: []Col{{Weight: 1, Items: []Item{{Type: "space", Height: 60}}}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	reparto := func(im image.Image) (arriba, abajo int) {
		g := im.(*image.Gray)
		b := g.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				if g.Pix[y*g.Stride+x] < 128 {
					if y < b.Dy()/2 {
						arriba++
					} else {
						abajo++
					}
				}
			}
		}
		return
	}

	a1, b1 := reparto(img)
	if a1 <= b1 {
		t.Fatalf("sin girar la tinta deberia estar arriba (arriba=%d abajo=%d)", a1, b1)
	}

	girada := Girar180(img)
	a2, b2 := reparto(girada)
	if b2 <= a2 {
		t.Errorf("al girar la tinta deberia quedar abajo (arriba=%d abajo=%d)", a2, b2)
	}
	if a1+b1 != a2+b2 {
		t.Errorf("se perdieron puntos al girar: %d antes, %d despues", a1+b1, a2+b2)
	}
	if girada.Bounds() != img.Bounds() {
		t.Errorf("cambio el tamano: %v -> %v", img.Bounds(), girada.Bounds())
	}
	t.Logf("sin girar arriba=%d abajo=%d ; girada arriba=%d abajo=%d", a1, b1, a2, b2)
}

// TestGirarDosVecesVuelveAlOrigen: una comprobacion barata de que el giro
// no deforma ni desplaza nada.
func TestGirarDosVecesVuelveAlOrigen(t *testing.T) {
	img, err := Render(Layout{Width: 384, Padding: 3, Rows: []Row{
		{Cols: []Col{
			{Weight: 1, Items: []Item{{Text: "izquierda"}}},
			{Dots: 120, Align: "right", Items: []Item{{QR: "https://x"}}},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	ida := Girar180(img)
	vuelta := Girar180(ida)

	g := img.(*image.Gray)
	for y := 0; y < g.Bounds().Dy(); y++ {
		for x := 0; x < g.Bounds().Dx(); x++ {
			if g.Pix[y*g.Stride+x] != vuelta.Pix[y*vuelta.Stride+x] {
				t.Fatalf("girar dos veces no devuelve la imagen original: difiere en (%d,%d)", x, y)
			}
		}
	}
}
