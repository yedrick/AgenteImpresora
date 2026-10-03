package layout

import (
	"image"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Las fuentes Go vienen dentro de golang.org/x/image, con licencia Apache 2.0:
// no hay que distribuir ningun archivo aparte y cubren acentos y la enye.
//
// A 203 ppp, que es la resolucion de practicamente toda termica de ticket,
// estos tamanos se leen bien:
var sizeTable = map[string]int{
	"xs":  16,
	"s":   20,
	"m":   26, // cuerpo normal
	"l":   34,
	"xl":  46,
	"xxl": 62,
}

const defaultSizePx = 26

// faceKey identifica una combinacion de fuente y tamano.
type faceKey struct {
	px   int
	bold bool
	mono bool
}

var (
	parsedMu sync.Mutex
	parsed   = map[string][]byte{}
	fonts    = map[string]*opentype.Font{}
)

// fontFor devuelve la fuente ya analizada. Compartirla entre goroutines es
// seguro: sfnt.Font lo es mientras cada cara tenga su propio buffer, y cada
// opentype.Face lleva el suyo.
func fontFor(name string, data []byte) *opentype.Font {
	parsedMu.Lock()
	defer parsedMu.Unlock()
	if f, ok := fonts[name]; ok {
		return f
	}
	f, err := opentype.Parse(data)
	if err != nil {
		// Las fuentes van incrustadas en el binario: si no se pueden leer,
		// el binario esta corrupto y no hay nada sensato que hacer.
		panic("layout: no se pudo leer la fuente " + name + ": " + err.Error())
	}
	fonts[name] = f
	parsed[name] = data
	return f
}

// fontSet son las caras de UN render. No se comparten entre goroutines.
//
// Antes habia una cache global de font.Face y eso era una carrera de datos:
// opentype.Face guarda estado interno (buffer de glifos y metricas
// perezosas), asi que dos workers componiendo a la vez se pisaban y el
// ticket podia salir con el texto corrupto. Crear una cara cuesta 713 ns
// frente a los 445 us de componer un bloque, asi que no compartirlas no se
// nota.
type fontSet struct {
	caras map[faceKey]font.Face
}

func newFontSet() *fontSet {
	return &fontSet{caras: map[faceKey]font.Face{}}
}

func (fs *fontSet) face(k faceKey) font.Face {
	if f, ok := fs.caras[k]; ok {
		return f
	}
	var data []byte
	var name string
	switch {
	case k.mono && k.bold:
		data, name = gomonobold.TTF, "gomonobold"
	case k.mono:
		data, name = gomono.TTF, "gomono"
	case k.bold:
		data, name = gobold.TTF, "gobold"
	default:
		data, name = goregular.TTF, "goregular"
	}
	face, err := opentype.NewFace(fontFor(name, data), &opentype.FaceOptions{
		Size:    float64(k.px),
		DPI:     72, // Size ya va en pixeles, asi que 72 ppp lo deja 1:1
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("layout: no se pudo crear la fuente: " + err.Error())
	}
	fs.caras[k] = face
	return face
}

// close libera las caras del render.
func (fs *fontSet) close() {
	for _, f := range fs.caras {
		_ = f.Close()
	}
	fs.caras = nil
}

func keyFor(it Item) *faceKey {
	px := it.SizePx
	if px <= 0 {
		if v, ok := sizeTable[strings.ToLower(strings.TrimSpace(it.Size))]; ok {
			px = v
		}
	}
	if px <= 0 {
		px = defaultSizePx
	}
	if px < 8 {
		px = 8
	}
	if px > 200 {
		px = 200
	}
	return &faceKey{px: px, bold: it.Bold, mono: it.Mono}
}

// lineHeight es el alto de una linea con esa fuente.
func (fs *fontSet) lineHeight(k *faceKey) int {
	m := fs.face(*k).Metrics()
	h := (m.Height.Ceil()*11 + 5) / 10 // un 10% de aire entre lineas
	if h < 1 {
		h = 1
	}
	return h
}

func (fs *fontSet) textWidth(k *faceKey, s string) int {
	return font.MeasureString(fs.face(*k), s).Ceil()
}

// wrapText parte el texto para que quepa en el ancho dado, cortando por
// espacios y, si una palabra sola no cabe, por caracteres.
func (fs *fontSet) wrapText(k *faceKey, text string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, parrafo := range strings.Split(text, "\n") {
		palabras := strings.Fields(parrafo)
		if len(palabras) == 0 {
			out = append(out, "")
			continue
		}
		linea := ""
		for _, p := range palabras {
			prueba := p
			if linea != "" {
				prueba = linea + " " + p
			}
			if fs.textWidth(k, prueba) <= width {
				linea = prueba
				continue
			}
			if linea != "" {
				out = append(out, linea)
			}
			// Una palabra que no cabe entera se parte por caracteres.
			if fs.textWidth(k, p) > width {
				trozo := ""
				for _, r := range p {
					if fs.textWidth(k, trozo+string(r)) > width && trozo != "" {
						out = append(out, trozo)
						trozo = ""
					}
					trozo += string(r)
				}
				linea = trozo
				continue
			}
			linea = p
		}
		if linea != "" {
			out = append(out, linea)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

// drawText pinta una linea en el lienzo y devuelve el rectangulo ocupado.
func (fs *fontSet) drawText(c *canvas, k *faceKey, s string, x, y, width int, align string) (int, int) {
	face := fs.face(*k)
	w := fs.textWidth(k, s)
	dx := 0
	switch align {
	case "center":
		dx = (width - w) / 2
	case "right":
		dx = width - w
	}
	if dx < 0 {
		dx = 0
	}

	// Se dibuja en una mascara propia y luego se mezcla, para que el
	// antialias no pise lo que ya hubiera debajo.
	m := face.Metrics()
	alto := fs.lineHeight(k)
	mask := image.NewGray(image.Rect(0, 0, maxInt(w, 1), maxInt(alto, 1)))
	for i := range mask.Pix {
		mask.Pix[i] = 255
	}
	d := font.Drawer{
		Dst:  mask,
		Src:  image.NewUniform(blackColor{}),
		Face: face,
		Dot:  fixed.Point26_6{X: 0, Y: m.Ascent},
	}
	d.DrawString(s)
	// Recortado al ancho: un glifo mas ancho que su columna se pintaba
	// encima de la vecina.
	c.blitClip(mask, x+dx, y, width-dx)
	return w, alto
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// blackColor es tinta opaca. Se define aqui para no arrastrar image/color
// solo por esto.
type blackColor struct{}

func (blackColor) RGBA() (r, g, b, a uint32) { return 0, 0, 0, 0xffff }
