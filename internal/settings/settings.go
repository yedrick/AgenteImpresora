package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	DefaultPrinter string  `json:"default_printer"`
	PaperWidth     int     `json:"paper_width"`
	ImageScale     int     `json:"image_scale"`
	Aliases        []Alias `json:"aliases"`
}

type Alias struct {
	Name        string `json:"name"`
	Printer     string `json:"printer"`
	Description string `json:"description,omitempty"`
}

func Default() Settings {
	return Settings{
		PaperWidth: 384,
		ImageScale: 80,
	}
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
	return cfg, os.WriteFile(path, b, 0o644)
}

func normalize(cfg Settings) Settings {
	// Se aceptan los tres anchos que el resto del agente sabe manejar
	// (printWidth y ticketCols). Antes cualquier valor distinto de 576 se
	// degradaba a 384, asi que 512 era inalcanzable.
	switch cfg.PaperWidth {
	case 384, 512, 576:
	default:
		cfg.PaperWidth = 384
	}
	if cfg.ImageScale <= 0 {
		cfg.ImageScale = 80
	}
	if cfg.ImageScale < 35 {
		cfg.ImageScale = 35
	}
	if cfg.ImageScale > 100 {
		cfg.ImageScale = 100
	}
	if len(cfg.Aliases) == 0 {
		cfg.Aliases = []Alias{
			{Name: "cocina", Printer: cfg.DefaultPrinter, Description: "Pedidos de cocina"},
			{Name: "recepcion", Printer: cfg.DefaultPrinter, Description: "Recepcion y caja"},
			{Name: "facturas", Printer: cfg.DefaultPrinter, Description: "Facturas"},
			{Name: "pagos", Printer: cfg.DefaultPrinter, Description: "Pagos y recibos"},
		}
	}
	return cfg
}
