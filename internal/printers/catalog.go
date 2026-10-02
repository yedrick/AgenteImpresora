package printers

import "strings"

// Catalogo de modelos conocidos.
//
// Lo que resuelve: casi ninguna termica ESC/POS necesita el driver del
// fabricante (por red, serie o Bluetooth no hace falta ninguno, y por USB en
// Windows suele bastar "Generic / Text Only"). Lo que si cambia de un modelo
// a otro, y hace que el ticket salga bien o salga torcido, es el ancho de
// papel, si tiene cuchilla y si entiende el comando de imagen moderno.
//
// Por eso esto es un catalogo de CAPACIDADES, no de drivers. Para el unico
// caso que de verdad necesita uno se indica la pagina oficial, nada se
// descarga ni se instala solo.

// DriverNeed dice cuando hace falta el driver del fabricante.
type DriverNeed string

const (
	// DriverNunca: funciona sin driver del fabricante en cualquier conexion.
	DriverNunca DriverNeed = "nunca"
	// DriverUSBWindows: por USB en Windows hace falta una cola de impresion;
	// el driver generico del sistema suele servir.
	DriverUSBWindows DriverNeed = "usb-windows"
	// DriverSiempre: el modelo exige el driver del fabricante por USB.
	DriverSiempre DriverNeed = "si"
)

// Model describe un modelo de impresora y lo que sabe hacer.
type Model struct {
	ID    string `json:"id"`
	Brand string `json:"brand"`
	Name  string `json:"name"`

	// Match son trozos del nombre con los que se reconoce la impresora tal
	// como la ve el sistema operativo.
	Match []string `json:"-"`

	// PaperWidth es el ancho en puntos por defecto, y Widths los que admite.
	PaperWidth int   `json:"paper_width"`
	Widths     []int `json:"widths"`

	// Cut es el modo de corte recomendado: partial, full o none.
	Cut string `json:"cut"`
	// Drawer indica si lleva conector para el cajon de dinero.
	Drawer bool `json:"drawer"`
	// Raster indica si entiende GS v 0, el comando de imagen moderno.
	Raster bool `json:"raster"`

	// Notes es lo que conviene saber antes de conectarla.
	Notes string `json:"notes,omitempty"`

	Driver    DriverNeed `json:"driver"`
	DriverURL string     `json:"driver_url,omitempty"`
}

// Columns son las columnas de texto con la fuente normal.
func (m Model) Columns() int {
	switch {
	case m.PaperWidth >= 576:
		return 48
	case m.PaperWidth >= 512:
		return 42
	default:
		return 32
	}
}

const (
	anchoEpson    = "https://download.epson-biz.com/modules/pos/"
	anchoStar     = "https://starmicronics.com/support/"
	anchoBixolon  = "https://bixolonusa.com/support/downloads/"
	anchoXprinter = "https://www.xprintertech.com/download.html"
	anchoGainscha = "https://www.gainscha.com/download"
	ancho3nStar   = "https://3nstar.com/home/support/"
	anchoRongta   = "https://www.rongtatech.com/"
)

// Catalog son los modelos reconocidos. Es deliberadamente corto: cubre la
// mayor parte del mercado de punto de venta sin inventar datos de modelos
// que no se han podido contrastar. Si tu impresora no esta, usa el generico
// de su ancho: el resto del agente funciona igual.
var Catalog = []Model{
	// --- Epson ---------------------------------------------------------
	{
		ID: "epson-tm-t20", Brand: "Epson", Name: "TM-T20 / T20II / T20III / T20X",
		Match:      []string{"tm-t20", "tm t20", "tmt20"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver:    DriverUSBWindows,
		DriverURL: anchoEpson,
		Notes:     "Por USB en Windows basta con el driver 'Generic / Text Only' del sistema; el APD de Epson es opcional. Por red o serie no hace falta ninguno.",
	},
	{
		ID: "epson-tm-t82", Brand: "Epson", Name: "TM-T82 / T82II / T82III",
		Match:      []string{"tm-t82", "tm t82"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoEpson,
	},
	{
		ID: "epson-tm-t88", Brand: "Epson", Name: "TM-T88 IV / V / VI / VII",
		Match:      []string{"tm-t88", "tm t88"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoEpson,
	},
	{
		ID: "epson-tm-m30", Brand: "Epson", Name: "TM-m30 / m30II / m30III",
		Match:      []string{"tm-m30", "tm m30"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoEpson,
		Notes: "Lleva Bluetooth y WiFi segun version. Por red no necesita driver.",
	},
	{
		ID: "epson-tm-u220", Brand: "Epson", Name: "TM-U220 (matricial)",
		Match:      []string{"tm-u220", "tm u220"},
		PaperWidth: 384, Widths: []int{384}, Cut: "partial", Drawer: true, Raster: false,
		Driver: DriverUSBWindows, DriverURL: anchoEpson,
		Notes: "NO es termica: es de impacto, con cinta. No entiende el comando de imagen moderno, asi que logos y QR no salen o salen muy lentos. Para tickets de texto va bien.",
	},

	// --- Star ----------------------------------------------------------
	{
		ID: "star-tsp100", Brand: "Star Micronics", Name: "TSP100 / TSP143 (futurePRNT)",
		Match:      []string{"tsp100", "tsp143", "tsp 100", "tsp 143"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverSiempre, DriverURL: anchoStar,
		Notes: "Caso especial. Por USB en Windows exige el driver futurePRNT de Star. Ademas, de fabrica NO habla ESC/POS: hay que activar la emulacion en la 'TSP100 Configuration Utility' antes de que este agente pueda imprimir. Las versiones LAN si funcionan por tcp:// sin driver.",
	},
	{
		ID: "star-tsp650", Brand: "Star Micronics", Name: "TSP650 / TSP650II",
		Match:      []string{"tsp650", "tsp 650"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoStar,
		Notes: "Viene en modo Star Line. Hay que ponerla en emulacion ESC/POS con los microinterruptores o la utilidad de configuracion.",
	},
	{
		ID: "star-mcprint3", Brand: "Star Micronics", Name: "mC-Print3",
		Match:      []string{"mc-print3", "mcprint3"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoStar,
		Notes: "Admite emulacion ESC/POS. Por red no necesita driver.",
	},

	// --- Bixolon -------------------------------------------------------
	{
		ID: "bixolon-srp-350", Brand: "Bixolon", Name: "SRP-350 / 350II / 350III / 350plus",
		Match:      []string{"srp-350", "srp350"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoBixolon,
	},
	{
		ID: "bixolon-srp-330", Brand: "Bixolon", Name: "SRP-330 / 330II",
		Match:      []string{"srp-330", "srp330"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoBixolon,
	},
	{
		ID: "bixolon-srp-e300", Brand: "Bixolon", Name: "SRP-E300",
		Match:      []string{"srp-e300", "srpe300"},
		PaperWidth: 576, Widths: []int{576}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoBixolon,
	},

	// --- Xprinter ------------------------------------------------------
	{
		ID: "xprinter-58", Brand: "Xprinter", Name: "XP-58 (58IIH / 58IIL / T58)",
		Match:      []string{"xp-58", "xp58", "pos58", "pos-58"},
		PaperWidth: 384, Widths: []int{384}, Cut: "none", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoXprinter,
		Notes: "58 mm, 384 puntos por linea. Muchas variantes de 58 mm no llevan cuchilla: si la tuya si, cambia el corte a parcial.",
	},
	{
		ID: "xprinter-80", Brand: "Xprinter", Name: "XP-80 / XP-80C / XP-Q200",
		Match:      []string{"xp-80", "xp80", "pos80", "pos-80", "xp-q200"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoXprinter,
	},

	// --- Gprinter / Gainscha --------------------------------------------
	{
		ID: "gprinter-58", Brand: "Gprinter", Name: "GP-58 (serie 58 mm)",
		Match:      []string{"gp-58", "gp58"},
		PaperWidth: 384, Widths: []int{384}, Cut: "none", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoGainscha,
		Notes: "Si tu modelo lleva cuchilla, cambia el corte a parcial.",
	},
	{
		ID: "gprinter-80", Brand: "Gprinter", Name: "GP-80 (serie 80 mm)",
		Match:      []string{"gp-80", "gp80", "gp-l80"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoGainscha,
	},

	// --- Otros ----------------------------------------------------------
	{
		ID: "3nstar-rpt", Brand: "3nStar", Name: "RPT-008 / RPT-010",
		Match:      []string{"rpt-008", "rpt008", "rpt-010", "rpt010", "3nstar"},
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows, DriverURL: ancho3nStar,
	},
	{
		ID: "rongta-rp58", Brand: "Rongta", Name: "RP58 (serie 58 mm)",
		Match:      []string{"rp58", "rp-58", "rongta"},
		PaperWidth: 384, Widths: []int{384}, Cut: "none", Drawer: false, Raster: true,
		Driver: DriverUSBWindows, DriverURL: anchoRongta,
	},

	// --- Genericos -------------------------------------------------------
	{
		ID: "generico-58", Brand: "Generico", Name: "Termica 58 mm",
		PaperWidth: 384, Widths: []int{384}, Cut: "none", Drawer: true, Raster: true,
		Driver: DriverUSBWindows,
		Notes:  "Punto de partida para cualquier termica de 58 mm. Si imprime bien pero no corta, tu modelo no lleva cuchilla.",
	},
	{
		ID: "generico-72", Brand: "Generico", Name: "Termica 72 mm",
		PaperWidth: 512, Widths: []int{512}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows,
	},
	{
		ID: "generico-80", Brand: "Generico", Name: "Termica 80 mm",
		PaperWidth: 576, Widths: []int{576, 384}, Cut: "partial", Drawer: true, Raster: true,
		Driver: DriverUSBWindows,
		Notes:  "Punto de partida para cualquier termica de 80 mm.",
	},
}

// MatchModel reconoce el modelo a partir del nombre que da el sistema.
// Devuelve false si no lo encuentra: el agente funciona igual, solo que sin
// los valores sugeridos.
func MatchModel(name string) (Model, bool) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == "" {
		return Model{}, false
	}
	// Se busca la coincidencia mas larga, para que "tm-t88" gane sobre un
	// hipotetico "tm-t8".
	best := Model{}
	bestLen := 0
	for _, m := range Catalog {
		for _, frag := range m.Match {
			if strings.Contains(lower, frag) && len(frag) > bestLen {
				best, bestLen = m, len(frag)
			}
		}
	}
	return best, bestLen > 0
}

// ModelByID busca un modelo por su identificador.
func ModelByID(id string) (Model, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, m := range Catalog {
		if m.ID == id {
			return m, true
		}
	}
	return Model{}, false
}
