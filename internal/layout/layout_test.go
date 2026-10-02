package layout

import (
	"image"
	"strings"
	"testing"
)

func render(t *testing.T, l Layout) *image.Gray {
	t.Helper()
	img, err := Render(l)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	g, ok := img.(*image.Gray)
	if !ok {
		t.Fatalf("Render deberia devolver *image.Gray, dio %T", img)
	}
	return g
}

// tinta cuenta los puntos negros de una zona, para comprobar que algo se
// dibujo donde toca.
func tinta(g *image.Gray, x, y, w, h int) int {
	n := 0
	b := g.Bounds()
	for yy := y; yy < y+h && yy < b.Dy(); yy++ {
		for xx := x; xx < x+w && xx < b.Dx(); xx++ {
			if g.Pix[yy*g.Stride+xx] < 128 {
				n++
			}
		}
	}
	return n
}

func TestAnchoExacto(t *testing.T) {
	for _, w := range []int{384, 512, 576} {
		g := render(t, Layout{Width: w, Rows: []Row{
			{Cols: []Col{{Items: []Item{{Text: "hola"}}}}},
		}})
		if got := g.Bounds().Dx(); got != w {
			t.Fatalf("ancho %d, esperaba %d", got, w)
		}
	}
}

// Lo que pedia el caso de uso: texto a la izquierda y un QR pequeno a la
// derecha, en la misma franja de papel.
func TestQRAlCostadoDelTexto(t *testing.T) {
	g := render(t, Layout{
		Width: 576,
		Rows: []Row{{
			Cols: []Col{
				{Weight: 1, Items: []Item{{Text: "Factura #F-000123"}, {Text: "Cliente: Ana"}}},
				{Dots: 140, Items: []Item{{QR: "https://ejemplo.com/f/123"}}},
			},
		}},
	})
	alto := g.Bounds().Dy()
	izquierda := tinta(g, 0, 0, 380, alto)
	derecha := tinta(g, 430, 0, 146, alto)
	if izquierda == 0 {
		t.Fatal("no se dibujo el texto de la izquierda")
	}
	if derecha == 0 {
		t.Fatal("no se dibujo el QR de la derecha")
	}
	// El QR es mucho mas denso que el texto: si no lo fuera, es que algo se
	// dibujo donde no toca.
	if derecha < izquierda/4 {
		t.Fatalf("la zona del QR tiene poca tinta (%d) frente al texto (%d)", derecha, izquierda)
	}
}

// Una columna con Dots fijos debe medir exactamente eso, y el resto repartir
// lo que sobra.
func TestRepartoDeAnchos(t *testing.T) {
	cero := 0
	l := Layout{Width: 576, Rows: []Row{{
		Gap: &cero,
		Cols: []Col{
			{Dots: 100, Items: []Item{{Text: "a"}}},
			{Weight: 1, Items: []Item{{Text: "b"}}},
			{Weight: 3, Items: []Item{{Text: "c"}}},
		},
	}}}
	m, err := measureRow(l.Rows[0], 576)
	if err != nil {
		t.Fatal(err)
	}
	if m.widths[0] != 100 {
		t.Fatalf("la columna fija mide %d, esperaba 100", m.widths[0])
	}
	suma := m.widths[0] + m.widths[1] + m.widths[2]
	if suma != 576 {
		t.Fatalf("las columnas suman %d, el ancho es 576", suma)
	}
	// La de peso 3 debe ser unas tres veces la de peso 1.
	if ratio := float64(m.widths[2]) / float64(m.widths[1]); ratio < 2.8 || ratio > 3.2 {
		t.Fatalf("proporcion %0.2f entre pesos 3 y 1, esperaba ~3", ratio)
	}
}

func TestTextoLargoSeParteEnVariasLineas(t *testing.T) {
	corto := render(t, Layout{Width: 576, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Text: "corto"}}}}},
	}})
	largo := render(t, Layout{Width: 576, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Text: strings.Repeat("palabra ", 40)}}}}},
	}})
	if largo.Bounds().Dy() <= corto.Bounds().Dy() {
		t.Fatalf("un texto largo deberia ocupar mas alto: %d vs %d",
			largo.Bounds().Dy(), corto.Bounds().Dy())
	}
}

// Una palabra sola mas ancha que la columna tiene que partirse igualmente, o
// se saldria del papel.
func TestPalabraMasAnchaQueLaColumna(t *testing.T) {
	k := keyFor(Item{Size: "m"})
	lineas := wrapText(k, strings.Repeat("X", 200), 200)
	if len(lineas) < 2 {
		t.Fatalf("deberia partirse en varias lineas, dio %d", len(lineas))
	}
	for _, l := range lineas {
		if w := textWidth(k, l); w > 200 {
			t.Fatalf("la linea %q mide %d, el limite son 200", l, w)
		}
	}
}

func TestLosTamanosDeFuenteEscalan(t *testing.T) {
	anterior := 0
	for _, size := range []string{"xs", "s", "m", "l", "xl", "xxl"} {
		h := lineHeight(keyFor(Item{Size: size}))
		if h <= anterior {
			t.Fatalf("el tamano %q da alto %d, no crece sobre el anterior %d", size, h, anterior)
		}
		anterior = h
	}
}

func TestAcentosYEnye(t *testing.T) {
	k := keyFor(Item{})
	con := textWidth(k, "Peña")
	sin := textWidth(k, "Pena")
	if con == 0 {
		t.Fatal("no se midio el texto con enye")
	}
	// No tienen por que medir igual, pero si parecido: si la enye no
	// existiera en la fuente, saldria un hueco o un cuadro.
	if con < sin*8/10 || con > sin*13/10 {
		t.Fatalf("ancho con enye %d frente a sin %d: la fuente no la dibuja bien", con, sin)
	}
}

func TestAlineacionHorizontal(t *testing.T) {
	alto := 0
	izq := render(t, Layout{Width: 400, Rows: []Row{
		{Cols: []Col{{Align: "left", Items: []Item{{Text: "X"}}}}},
	}})
	alto = izq.Bounds().Dy()
	der := render(t, Layout{Width: 400, Rows: []Row{
		{Cols: []Col{{Align: "right", Items: []Item{{Text: "X"}}}}},
	}})
	if tinta(izq, 0, 0, 100, alto) == 0 {
		t.Fatal("alineado a la izquierda, no hay tinta en el borde izquierdo")
	}
	if tinta(izq, 300, 0, 100, alto) != 0 {
		t.Fatal("alineado a la izquierda, no deberia haber tinta a la derecha")
	}
	if tinta(der, 300, 0, 100, der.Bounds().Dy()) == 0 {
		t.Fatal("alineado a la derecha, no hay tinta en el borde derecho")
	}
}

func TestBordes(t *testing.T) {
	g := render(t, Layout{Width: 200, Border: true, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Text: "x"}}}}},
	}})
	// Las cuatro esquinas deben tener tinta.
	w, h := g.Bounds().Dx(), g.Bounds().Dy()
	for _, p := range [][2]int{{0, 0}, {w - 2, 0}, {0, h - 2}, {w - 2, h - 2}} {
		if tinta(g, p[0], p[1], 2, 2) == 0 {
			t.Fatalf("falta el marco en la esquina (%d,%d)", p[0], p[1])
		}
	}
}

func TestInvertido(t *testing.T) {
	normal := render(t, Layout{Width: 300, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Text: "TOTAL", Size: "l"}}}}},
	}})
	invertido := render(t, Layout{Width: 300, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Text: "TOTAL", Size: "l", Invert: true}}}}},
	}})
	tn := tinta(normal, 0, 0, 300, normal.Bounds().Dy())
	ti := tinta(invertido, 0, 0, 300, invertido.Bounds().Dy())
	if ti <= tn {
		t.Fatalf("el invertido deberia tener mucha mas tinta: %d frente a %d", ti, tn)
	}
}

func TestCodigoDeBarras(t *testing.T) {
	g := render(t, Layout{Width: 576, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Barcode: "F000123", BarHeight: 60}}}}},
	}})
	if tinta(g, 0, 0, 576, g.Bounds().Dy()) == 0 {
		t.Fatal("no se dibujo el codigo de barras")
	}
}

// El digito de control de Code128 es lo que valida el lector: si estuviera
// mal, el codigo no se leeria.
func TestDigitoDeControlCode128(t *testing.T) {
	// Para "A" (valor 33) con inicio B (104): 104 + 1*33 = 137; 137 % 103 = 34.
	vals, err := encodeCode128B("A")
	if err != nil {
		t.Fatal(err)
	}
	if len(vals) != 4 {
		t.Fatalf("esperaba inicio, dato, control y fin; dio %v", vals)
	}
	if vals[0] != code128StartB || vals[1] != 33 || vals[2] != 34 || vals[3] != code128Stop {
		t.Fatalf("secuencia %v incorrecta", vals)
	}
}

func TestCode128RechazaLoQueNoPuedeCodificar(t *testing.T) {
	if _, err := encodeCode128B("café"); err == nil {
		t.Fatal("un caracter fuera de ASCII deberia dar error en vez de salir mal impreso")
	}
	if _, err := encodeCode128B(""); err == nil {
		t.Fatal("un codigo vacio deberia dar error")
	}
}

// El QR debe caber en su columna: si se sale, la impresora lo recorta y deja
// de leerse.
func TestElQRCabeEnSuColumna(t *testing.T) {
	for _, dots := range []int{80, 120, 200} {
		img, err := renderQR("https://kollatek.com/f/000123", 0, dots, "M")
		if err != nil {
			t.Fatal(err)
		}
		if w := img.Bounds().Dx(); w > dots {
			t.Fatalf("con %d puntos disponibles el QR mide %d", dots, w)
		}
		if img.Bounds().Dx() != img.Bounds().Dy() {
			t.Fatal("un QR debe ser cuadrado")
		}
	}
}

// Sin zona de silencio los lectores fallan.
func TestElQRLlevaZonaDeSilencio(t *testing.T) {
	img, err := renderQR("hola", 4, 0, "M")
	if err != nil {
		t.Fatal(err)
	}
	g := img.(*image.Gray)
	// Los bordes tienen que estar limpios.
	if tinta(g, 0, 0, g.Bounds().Dx(), 4) != 0 {
		t.Fatal("el borde superior del QR deberia estar en blanco")
	}
	if tinta(g, 0, 0, 4, g.Bounds().Dy()) != 0 {
		t.Fatal("el borde izquierdo del QR deberia estar en blanco")
	}
}

func TestErroresUtiles(t *testing.T) {
	if _, err := Render(Layout{Width: 10, Padding: 20, Rows: []Row{{Cols: []Col{{}}}}}); err == nil {
		t.Fatal("un bloque sin ancho util deberia dar error")
	}
	_, err := Render(Layout{Width: 576, Rows: []Row{
		{Cols: []Col{{Items: []Item{{Type: "image", Image: "no-es-base64"}}}}},
	}})
	if err == nil {
		t.Fatal("una imagen invalida deberia dar error")
	}
	if !strings.Contains(err.Error(), "fila 1") {
		t.Fatalf("el error deberia decir donde esta el problema: %v", err)
	}
}

// Un bloque gigante gasta papel y puede desbordar el buffer de la impresora.
func TestAlturaMaxima(t *testing.T) {
	var rows []Row
	for i := 0; i < 300; i++ {
		rows = append(rows, Row{Cols: []Col{{Items: []Item{{Text: "linea", Size: "xxl"}}}}})
	}
	if _, err := Render(Layout{Width: 576, Rows: rows}); err == nil {
		t.Fatal("un bloque por encima del maximo deberia rechazarse")
	}
}

// El tipo se puede deducir del campo que venga relleno.
func TestTipoDeducido(t *testing.T) {
	cases := map[string]Item{
		"qr":      {QR: "x"},
		"barcode": {Barcode: "x"},
		"image":   {Image: "x"},
		"text":    {Text: "x"},
		"rule":    {Type: "rule"},
	}
	for quiero, it := range cases {
		if got := itemType(it); got != quiero {
			t.Fatalf("%+v -> %q, esperaba %q", it, got, quiero)
		}
	}
}

// Con gap cero las columnas van pegadas; omitirlo deja la separacion por
// defecto. Con un int normal no habia forma de pedir lo primero.
func TestGapCeroSeDistingueDeNoIndicado(t *testing.T) {
	cero := 0
	cols := []Col{{Items: []Item{{Text: "a"}}}, {Items: []Item{{Text: "b"}}}}

	pegadas, err := measureRow(Row{Gap: &cero, Cols: cols}, 400)
	if err != nil {
		t.Fatal(err)
	}
	if suma := pegadas.widths[0] + pegadas.widths[1]; suma != 400 {
		t.Fatalf("con gap cero las columnas deberian sumar 400, suman %d", suma)
	}

	porDefecto, err := measureRow(Row{Cols: cols}, 400)
	if err != nil {
		t.Fatal(err)
	}
	if suma := porDefecto.widths[0] + porDefecto.widths[1]; suma != 400-defaultGap {
		t.Fatalf("sin indicar gap deberian sumar %d, suman %d", 400-defaultGap, suma)
	}
}
