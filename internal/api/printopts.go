// Opciones comunes a todos los trabajos de impresion y su resolucion contra
// la impresora dada de alta.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"collatech-agent/internal/escpos"
	"collatech-agent/internal/settings"
)

// CutSetting acepta tanto el booleano de siempre como el nombre del modo, de
// forma que `"cut": true` y `"cut": "partial"` siguen funcionando los dos.
type CutSetting struct {
	Mode escpos.CutMode
	Set  bool
}

func (c *CutSetting) UnmarshalJSON(b []byte) error {
	// null es "no indicado", no "no cortar". Sin esto, cualquier cliente que
	// serialice los opcionales ausentes como null (lo normal en C#, Java o
	// Python) dejaba de cortar los tickets para siempre y sin ningun aviso,
	// pisando ademas el modo de corte de la impresora.
	if string(bytes.TrimSpace(b)) == "null" {
		return nil
	}
	var asBool bool
	if err := json.Unmarshal(b, &asBool); err == nil {
		c.Set = true
		if asBool {
			c.Mode = escpos.CutPartial
		} else {
			c.Mode = escpos.CutNone
		}
		return nil
	}
	var asText string
	if err := json.Unmarshal(b, &asText); err != nil {
		return fmt.Errorf("cut debe ser true/false o uno de: partial, full, none")
	}
	switch strings.ToLower(strings.TrimSpace(asText)) {
	case "partial", "parcial":
		c.Mode = escpos.CutPartial
	case "full", "total", "completo":
		c.Mode = escpos.CutFull
	case "none", "no", "ninguno":
		c.Mode = escpos.CutNone
	case "":
		// Cadena vacia tambien es "no indicado".
		return nil
	default:
		return fmt.Errorf("cut %q no valido: usa partial, full o none", asText)
	}
	c.Set = true
	return nil
}

func (c CutSetting) MarshalJSON() ([]byte, error) {
	if !c.Set {
		return []byte(`null`), nil
	}
	return json.Marshal(string(c.Mode))
}

// docRequest son los campos que entiende cualquier endpoint de impresion.
// Se incrusta en cada peticion concreta.
type docRequest struct {
	Printer string `json:"printer"`

	// Width en puntos: 384, 512 o 576. Si se omite se toma el de la
	// impresora dada de alta, y si tampoco, el general.
	Width int `json:"width"`

	Cut        CutSetting `json:"cut"`
	Drawer     bool       `json:"drawer"`
	UpsideDown *bool      `json:"upside_down"`

	// LineSpacing en puntos. "compact" aprieta el ticket; 0 deja el de la
	// impresora.
	LineSpacing int    `json:"line_spacing"`
	Compact     bool   `json:"compact"`
	Font        string `json:"font"`

	FeedTop int `json:"feed_top"`
	// FeedBottom es puntero porque 0 es un valor valido y distinto de "no
	// indicado": quien pone 0 quiere cortar al limite, sin avance, aunque
	// se arriesgue a que la cuchilla toque la ultima linea.
	FeedBottom *int `json:"feed_bottom"`
	MarginDots int  `json:"margin_dots"`
}

// resolved es una peticion ya combinada con los ajustes de la impresora.
type resolved struct {
	Targets []string
	Profile settings.Printer
	Doc     escpos.DocOptions
	Scale   int
}

// resolve combina, por este orden: lo que trae la peticion, los ajustes de la
// impresora dada de alta y los generales. Asi se puede tener una de 58 mm en
// cocina y una de 80 mm en caja sin repetir el ancho en cada llamada.
func (s *Server) resolve(req docRequest) (resolved, error) {
	name := strings.TrimSpace(req.Printer)
	if name == "" {
		return resolved{}, fmt.Errorf("falta el nombre de la impresora")
	}

	general, err := s.loadSettings()
	if err != nil {
		s.logger.Error("settings_load", map[string]any{"error": err.Error()})
		general = settings.Default()
	}

	profile, known := general.Find(name)
	targets := []string{name}
	if known {
		targets = profile.Targets()
	}
	if len(targets) == 0 {
		return resolved{}, fmt.Errorf("la impresora %q no tiene ningun destino configurado", name)
	}

	width := req.Width
	if width <= 0 {
		width = profile.PaperWidth
	}
	if width <= 0 {
		width = general.PaperWidth
	}

	cut := escpos.CutPartial
	switch {
	case req.Cut.Set:
		cut = req.Cut.Mode
	case profile.Cut != "":
		cut = escpos.CutMode(profile.Cut)
	}

	spacing := req.LineSpacing
	if req.Compact && spacing == 0 {
		spacing = escpos.TightLineSpacing
	}
	if spacing == 0 {
		spacing = profile.LineSpacing
	}

	font := req.Font
	if font == "" {
		font = profile.Font
	}

	upside := profile.UpsideDown
	if req.UpsideDown != nil {
		upside = *req.UpsideDown
	}

	// Sin indicar nada manda el perfil de la impresora; si tampoco dice
	// nada, el documento usa su valor por defecto.
	// Por la API, "feed_bottom": 0 significa cortar al limite. Dentro se
	// traduce a SinAvance, porque ahi el 0 quiere decir "no indicado".
	feedBottom := 0
	if req.FeedBottom != nil {
		feedBottom = *req.FeedBottom
		if feedBottom == 0 {
			feedBottom = escpos.SinAvance
		}
	} else if profile.FeedBottom > 0 {
		feedBottom = profile.FeedBottom
	}

	return resolved{
		Targets: targets,
		Profile: profile,
		Scale:   general.ImageScale,
		Doc: escpos.DocOptions{
			PaperWidth:  printWidth(width),
			LineSpacing: spacing,
			UpsideDown:  upside,
			LeftMargin:  req.MarginDots,
			Font:        font,
			FeedTop:     req.FeedTop,
			FeedBottom:  feedBottom,
			Cut:         cut,
			Drawer:      req.Drawer,
		},
	}, nil
}
