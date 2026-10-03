package layout

import (
	"image"
	"testing"
)

// primeraTinta devuelve la primera columna de pixeles con tinta.
func primeraTinta(t *testing.T, img image.Image) int {
	t.Helper()
	g := img.(*image.Gray)
	b := g.Bounds()
	for x := 0; x < b.Dx(); x++ {
		for y := 0; y < b.Dy(); y++ {
			if g.Pix[y*g.Stride+x] < 128 {
				return x
			}
		}
	}
	return -1
}

// TestMargenPorLado: se puede pedir margen en un lado concreto, y un 0
// expreso pega el contenido al borde aunque Pad diga otra cosa.
//
// Antes solo existia Pad, el mismo por los cuatro lados: para separar algo
// de la izquierda habia que separarlo tambien de arriba y de la derecha.
func TestMargenPorLado(t *testing.T) {
	cero, veinte := 0, 20

	casos := []struct {
		nombre string
		col    Col
		quiero int
		exacto bool
	}{
		{"sin margen", Col{Weight: 1, Items: []Item{{Text: "H"}}}, 0, false},
		{"pad en los cuatro lados", Col{Weight: 1, Pad: 20, Items: []Item{{Text: "H"}}}, 20, false},
		{"solo a la izquierda", Col{Weight: 1, PadL: &veinte, Items: []Item{{Text: "H"}}}, 20, false},
		// Pad separa de todo, pero la izquierda se pega a proposito.
		{"izquierda a 0 manda sobre pad", Col{Weight: 1, Pad: 20, PadL: &cero, Items: []Item{{Text: "H"}}}, 0, false},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			img, err := Render(Layout{Width: 576, Padding: 0, Rows: []Row{{Cols: []Col{c.col}}}})
			if err != nil {
				t.Fatal(err)
			}
			x := primeraTinta(t, img)
			if x < 0 {
				t.Fatal("no se pinto nada")
			}
			// El glifo tiene su propio hueco a la izquierda, asi que se
			// comprueba que empiece a partir del margen pedido y no mucho
			// despues.
			if x < c.quiero || x > c.quiero+12 {
				t.Errorf("la tinta empieza en x=%d y se esperaba cerca de %d", x, c.quiero)
			}
			t.Logf("margen pedido %d -> primera tinta en x=%d", c.quiero, x)
		})
	}
}

// TestMargenArribaEmpujaHaciaAbajo comprueba el lado de arriba, que es el
// otro que se pidio.
func TestMargenArribaEmpujaHaciaAbajo(t *testing.T) {
	treinta := 30
	sin, err := Render(Layout{Width: 576, Padding: 0, Rows: []Row{{Cols: []Col{{Weight: 1, Items: []Item{{Text: "H"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	con, err := Render(Layout{Width: 576, Padding: 0, Rows: []Row{{Cols: []Col{{Weight: 1, PadT: &treinta, Items: []Item{{Text: "H"}}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	diff := con.Bounds().Dy() - sin.Bounds().Dy()
	if diff != treinta {
		t.Errorf("con 30 puntos de margen arriba el bloque crecio %d, se esperaban %d", diff, treinta)
	}
}

// TestPrimeraFilaPegadaArriba: con el margen de arriba a 0, la primera fila
// empieza en el borde aunque el bloque tenga margen por los demas lados.
//
// Antes el margen del bloque era uno solo para los cuatro lados: para que la
// primera fila no dejara hueco arriba habia que quitarlo tambien a los
// costados, y entonces el texto quedaba pegado al borde del papel.
func TestPrimeraFilaPegadaArriba(t *testing.T) {
	primeraFilaConTinta := func(img image.Image) int {
		g := img.(*image.Gray)
		b := g.Bounds()
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				if g.Pix[y*g.Stride+x] < 128 {
					return y
				}
			}
		}
		return -1
	}

	fila := []Row{{Cols: []Col{{Weight: 1, Items: []Item{{Text: "HOLA", Size: "xl"}}}}}}
	cero := 0

	conMargen, err := Render(Layout{Width: 576, Padding: 24, Rows: fila})
	if err != nil {
		t.Fatal(err)
	}
	pegado, err := Render(Layout{Width: 576, Padding: 24, PadTop: &cero, Rows: fila})
	if err != nil {
		t.Fatal(err)
	}

	yCon := primeraFilaConTinta(conMargen)
	yPeg := primeraFilaConTinta(pegado)
	if yCon < 24 {
		t.Errorf("con margen de 24 la tinta empieza en y=%d, deberia ser 24 o mas", yCon)
	}
	if yPeg >= 24 {
		t.Errorf("con el margen de arriba a 0 la tinta empieza en y=%d: sigue dejando hueco", yPeg)
	}
	t.Logf("margen normal -> y=%d ; arriba a 0 -> y=%d", yCon, yPeg)

	// Los costados tienen que conservar su margen.
	if primeraTinta(t, pegado) < 24 {
		t.Errorf("al quitar el margen de arriba tambien se perdio el de la izquierda")
	}
}
