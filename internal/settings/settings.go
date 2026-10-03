// Package settings guarda la configuracion que el usuario cambia desde el
// panel: impresoras dadas de alta y preferencias de impresion.
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Printer es una impresora dada de alta, con sus ajustes propios. Antes solo
// habia "alias" (un nombre y uno o varios destinos), asi que el ancho de
// papel, el modo de corte o el giro eran globales y no se podia tener una
// termica de 58 mm en cocina y una de 80 mm en caja.
type Printer struct {
	// Name es como se la llama al imprimir.
	Name string `json:"name"`
	// Target es uno o varios destinos separados por coma. Con varios, cada
	// impresion saca una copia en cada uno.
	Target string `json:"target"`
	// PaperWidth en puntos: 384, 512 o 576. Cero usa el valor general.
	PaperWidth int `json:"paper_width,omitempty"`
	// Cut: partial, full o none.
	Cut string `json:"cut,omitempty"`
	// UpsideDown gira el ticket 180 grados en esta impresora.
	UpsideDown bool `json:"upside_down,omitempty"`
	// LineSpacing en puntos; 0 deja el de fabrica.
	LineSpacing int `json:"line_spacing,omitempty"`
	// Font: "a" normal, "b" condensada.
	Font string `json:"font,omitempty"`
	// FeedBottom son las lineas que se avanzan al cortar en esta impresora.
	FeedBottom int `json:"feed_bottom,omitempty"`
	// Model es el identificador del catalogo, si se eligio uno. Se guarda
	// para que el panel pueda volver a mostrarlo; los ajustes concretos ya
	// quedan copiados en los campos de arriba.
	Model string `json:"model,omitempty"`
	// Description es para que el operador sepa cual es.
	Description string `json:"description,omitempty"`
}

// Alias se mantiene por compatibilidad con la configuracion anterior; al
// cargar se convierte en Printer.
type Alias struct {
	Name        string `json:"name"`
	Printer     string `json:"printer"`
	Description string `json:"description,omitempty"`
}

type Settings struct {
	DefaultPrinter string    `json:"default_printer"`
	PaperWidth     int       `json:"paper_width"`
	ImageScale     int       `json:"image_scale"`
	Printers       []Printer `json:"printers"`

	// Aliases solo se lee para migrar configuraciones antiguas.
	Aliases []Alias `json:"aliases,omitempty"`
}

func Default() Settings {
	return Settings{
		PaperWidth: 384,
		ImageScale: 80,
	}
}

// Find busca una impresora por nombre, sin distinguir mayusculas.
func (s Settings) Find(name string) (Printer, bool) {
	key := strings.ToLower(strings.TrimSpace(name))
	for _, p := range s.Printers {
		if strings.ToLower(p.Name) == key {
			return p, true
		}
	}
	return Printer{}, false
}

// Targets parte el destino de una impresora en sus destinos reales.
func (p Printer) Targets() []string {
	parts := strings.FieldsFunc(p.Target, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		t := strings.TrimSpace(part)
		key := strings.ToLower(t)
		if t == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

func Load(path string) (Settings, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	// Se quita el BOM: lo deja PowerShell al editar el archivo a mano.
	b = trimBOM(b)
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return normalize(cfg), nil
}

func Save(path string, cfg Settings) (Settings, error) {
	cfg = normalize(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return cfg, err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return cfg, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return cfg, err
	}
	return cfg, os.Rename(tmp, path)
}

func trimBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

// ValidPaperWidths son los anchos que el resto del agente sabe componer.
var ValidPaperWidths = []int{384, 512, 576}

func normalizePaperWidth(w int) int {
	for _, v := range ValidPaperWidths {
		if w == v {
			return w
		}
	}
	return 384
}

func normalize(cfg Settings) Settings {
	cfg.PaperWidth = normalizePaperWidth(cfg.PaperWidth)
	if cfg.ImageScale <= 0 {
		cfg.ImageScale = 80
	}
	if cfg.ImageScale < 35 {
		cfg.ImageScale = 35
	}
	if cfg.ImageScale > 100 {
		cfg.ImageScale = 100
	}

	// Migrar los alias antiguos a impresoras, sin pisar las que ya existan.
	for _, a := range cfg.Aliases {
		if a.Name == "" || a.Printer == "" {
			continue
		}
		if _, ok := cfg.Find(a.Name); ok {
			continue
		}
		cfg.Printers = append(cfg.Printers, Printer{
			Name:        strings.ToLower(strings.TrimSpace(a.Name)),
			Target:      strings.TrimSpace(a.Printer),
			Description: a.Description,
		})
	}
	cfg.Aliases = nil

	out := make([]Printer, 0, len(cfg.Printers))
	seen := map[string]bool{}
	for _, p := range cfg.Printers {
		p.Name = strings.ToLower(strings.TrimSpace(p.Name))
		p.Target = strings.TrimSpace(p.Target)
		if p.Name == "" || p.Target == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		if p.PaperWidth != 0 {
			p.PaperWidth = normalizePaperWidth(p.PaperWidth)
		}
		switch strings.ToLower(p.Cut) {
		case "full", "partial", "none":
			p.Cut = strings.ToLower(p.Cut)
		default:
			p.Cut = ""
		}
		if f := strings.ToLower(p.Font); f == "a" || f == "b" {
			p.Font = f
		} else {
			p.Font = ""
		}
		p.Model = strings.ToLower(strings.TrimSpace(p.Model))
		p.Description = strings.TrimSpace(p.Description)
		out = append(out, p)
	}
	cfg.Printers = out
	return cfg
}
