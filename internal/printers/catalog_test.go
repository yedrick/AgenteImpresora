package printers

import (
	"net/url"
	"strings"
	"testing"
)

// El catalogo existe para proponer valores correctos, asi que lo que mas
// importa es que esos valores sean coherentes.
func TestCatalogoCoherente(t *testing.T) {
	vistos := map[string]bool{}
	for _, m := range Catalog {
		if m.ID == "" || m.Brand == "" || m.Name == "" {
			t.Fatalf("modelo incompleto: %+v", m)
		}
		if vistos[m.ID] {
			t.Fatalf("id repetido: %s", m.ID)
		}
		vistos[m.ID] = true

		if m.PaperWidth != 384 && m.PaperWidth != 512 && m.PaperWidth != 576 {
			t.Fatalf("%s: ancho %d no es uno de los que compone el agente", m.ID, m.PaperWidth)
		}
		if len(m.Widths) == 0 {
			t.Fatalf("%s: sin anchos admitidos", m.ID)
		}
		var contiene bool
		for _, w := range m.Widths {
			if w == m.PaperWidth {
				contiene = true
			}
			if w != 384 && w != 512 && w != 576 {
				t.Fatalf("%s: ancho admitido %d invalido", m.ID, w)
			}
		}
		if !contiene {
			t.Fatalf("%s: el ancho por defecto %d no esta entre los admitidos %v", m.ID, m.PaperWidth, m.Widths)
		}
		switch m.Cut {
		case "partial", "full", "none":
		default:
			t.Fatalf("%s: modo de corte %q invalido", m.ID, m.Cut)
		}
		switch m.Driver {
		case DriverNunca, DriverUSBWindows, DriverSiempre:
		default:
			t.Fatalf("%s: valor de driver %q invalido", m.ID, m.Driver)
		}
		// Si hace falta driver siempre, hay que decir donde conseguirlo.
		if m.Driver == DriverSiempre && m.DriverURL == "" {
			t.Fatalf("%s: exige driver pero no indica donde descargarlo", m.ID)
		}
		if m.DriverURL != "" {
			u, err := url.Parse(m.DriverURL)
			if err != nil || u.Scheme != "https" {
				t.Fatalf("%s: el enlace del driver debe ser https valido, es %q", m.ID, m.DriverURL)
			}
		}
	}
}

// Las columnas tienen que cuadrar con lo que compone el resto del agente.
func TestColumnasDelModelo(t *testing.T) {
	for _, m := range Catalog {
		cols := m.Columns()
		esperado := map[int]int{384: 32, 512: 42, 576: 48}[m.PaperWidth]
		if cols != esperado {
			t.Fatalf("%s: %d puntos -> %d columnas, esperaba %d", m.ID, m.PaperWidth, cols, esperado)
		}
	}
}

func TestReconocerModeloPorNombre(t *testing.T) {
	cases := map[string]string{
		"EPSON TM-T20II Receipt":  "epson-tm-t20",
		"TM-T88V":                 "epson-tm-t88",
		"Star TSP143IIIU":         "star-tsp100",
		"TSP100 Cutter (TSP143)":  "star-tsp100",
		"BIXOLON SRP-350III":      "bixolon-srp-350",
		"XP-80C":                  "xprinter-80",
		"POS58 Printer":           "xprinter-58",
		"Gprinter GP-80250I":      "gprinter-80",
		"EPSON TM-U220B":          "epson-tm-u220",
		"Impresora que no existe": "",
		"":                        "",
	}
	for nombre, quiero := range cases {
		m, ok := MatchModel(nombre)
		if quiero == "" {
			if ok {
				t.Fatalf("%q no deberia reconocerse, dio %s", nombre, m.ID)
			}
			continue
		}
		if !ok || m.ID != quiero {
			t.Fatalf("%q -> %q (ok=%v), esperaba %q", nombre, m.ID, ok, quiero)
		}
	}
}

// Se elige la coincidencia mas larga para que un modelo concreto gane sobre
// uno mas generico.
func TestGanaLaCoincidenciaMasLarga(t *testing.T) {
	m, ok := MatchModel("Star TSP650II")
	if !ok || m.ID != "star-tsp650" {
		t.Fatalf("TSP650II -> %q, esperaba star-tsp650", m.ID)
	}
}

func TestModelPorID(t *testing.T) {
	if m, ok := ModelByID("generico-80"); !ok || m.PaperWidth != 576 {
		t.Fatalf("generico-80: %+v ok=%v", m, ok)
	}
	if _, ok := ModelByID("no-existe"); ok {
		t.Fatal("un id inventado no deberia encontrarse")
	}
}

// La TSP100 es el unico caso del catalogo que de verdad exige el driver del
// fabricante; si eso cambiara, el aviso del panel dejaria de tener sentido.
func TestLaTSP100AvisaDeSuCaso(t *testing.T) {
	m, ok := ModelByID("star-tsp100")
	if !ok {
		t.Fatal("falta la TSP100 en el catalogo")
	}
	if m.Driver != DriverSiempre {
		t.Fatalf("la TSP100 exige driver por USB en Windows, esta como %q", m.Driver)
	}
	if !strings.Contains(strings.ToLower(m.Notes), "emulaci") {
		t.Fatal("la nota deberia avisar de que hay que activar la emulacion ESC/POS")
	}
}

// La TM-U220 es de impacto: no entiende el comando de imagen moderno y eso
// hay que decirlo, o el cliente se vuelve loco con los logos.
func TestLaU220AvisaDeQueNoEsTermica(t *testing.T) {
	m, _ := ModelByID("epson-tm-u220")
	if m.Raster {
		t.Fatal("la TM-U220 no soporta GS v 0")
	}
	if !strings.Contains(strings.ToLower(m.Notes), "impacto") {
		t.Fatal("la nota deberia avisar de que no es termica")
	}
}
