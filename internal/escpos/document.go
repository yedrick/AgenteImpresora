package escpos

// DocOptions son los ajustes que valen para todo un ticket, no para una linea
// suelta. Antes cada endpoint construia su propio arranque y su propio cierre,
// asi que el corte, el avance y el interlineado no se comportaban igual segun
// por donde se imprimiera.
type DocOptions struct {
	// PaperWidth en puntos: 384 (58 mm), 512 (72 mm) o 576 (80 mm).
	PaperWidth int

	// LineSpacing es el alto de linea en puntos. 0 deja el de fabrica de la
	// impresora; TightLineSpacing aprieta el ticket y gasta menos papel.
	LineSpacing int

	// UpsideDown gira el ticket 180 grados, para las impresoras que expulsan
	// el papel hacia el operador y conviene leerlo con la cabecera abajo.
	UpsideDown bool

	// LeftMargin en puntos.
	LeftMargin int

	// Font es la fuente interna: "a" normal, "b" condensada.
	Font string

	// FeedTop son las lineas en blanco antes del contenido. Cero por
	// defecto: el espacio de arriba es papel desperdiciado en cada ticket.
	FeedTop int

	// FeedBottom son las lineas que se avanzan al cortar.
	//
	// 0 es "no indicado" y usa CutFeedLines, que es lo seguro: asi, un
	// DocOptions construido sin tocar este campo nunca corta texto. Para
	// pedir cero avance de verdad, cortando al limite, se usa SinAvance.
	// Por debajo de
	// CutFeedLines la cuchilla se come la ultima linea.
	FeedBottom int

	Cut    CutMode
	Drawer bool
}

// Columns son las columnas de texto que caben con la fuente normal.
func (o DocOptions) Columns() int {
	switch {
	case o.PaperWidth >= 576:
		return 48
	case o.PaperWidth >= 512:
		return 42
	default:
		return 32
	}
}

// Begin crea el documento y emite el arranque: reinicio, juego de caracteres
// y los ajustes que valen para todo el ticket.
func Begin(opt DocOptions) *Builder {
	b := New().Initialize()
	if opt.LineSpacing > 0 {
		b.LineSpacing(opt.LineSpacing)
	}
	if opt.LeftMargin > 0 {
		b.LeftMargin(opt.LeftMargin)
	}
	if opt.Font != "" {
		b.Font(opt.Font)
	}
	// El giro se activa antes de imprimir nada: afecta a lo que venga
	// despues, no a lo ya emitido.
	if opt.UpsideDown {
		b.UpsideDown(true)
	}
	if opt.Drawer {
		b.DrawerKick()
	}
	if opt.FeedTop > 0 {
		b.Feed(opt.FeedTop)
	}
	return b
}

// End cierra el documento: avanza lo justo, corta y deja la impresora en un
// estado limpio para el siguiente ticket.
func (b *Builder) End(opt DocOptions) *Builder {
	b.Bold(false).Underline(false).Inverted(false).TextScale(1, 1).AlignLeft()
	if opt.UpsideDown {
		b.UpsideDown(false)
	}
	if opt.LineSpacing > 0 {
		b.LineSpacing(0) // restaurar el valor de fabrica
	}
	if opt.Cut == CutNone || opt.Cut == "" {
		// Sin corte se avanza igualmente para que la ultima linea salga del
		// cabezal y se pueda leer.
		if opt.FeedBottom > 0 {
			b.Feed(opt.FeedBottom)
		}
		return b
	}
	// Sin indicar nada se avanzan CutFeedLines, que es lo que hace falta en
	// la mayoria para que la cuchilla no se coma la ultima linea.
	//
	// Pero si se indica un valor se respeta, aunque sea menor. Antes se
	// elevaba siempre al minimo y no habia forma de acortar el trozo de
	// papel en blanco del final: en una impresora cuya cuchilla queda mas
	// cerca del cabezal, 4 lineas sobran y se desperdicia papel en cada
	// ticket. Quien lo baja esta probando en su impresora; si se come una
	// linea, lo ve al momento y lo sube.
	feed := opt.FeedBottom
	switch {
	case feed == SinAvance:
		feed = 0 // cortar al limite, a peticion expresa
	case feed <= 0:
		feed = CutFeedLines
	}
	return b.Cut(opt.Cut, feed)
}
