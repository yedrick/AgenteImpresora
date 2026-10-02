package layout

import "image"

// canvas es un lienzo en escala de grises donde 0 es tinta y 255 es papel.
// Se trabaja sobre bytes y no sobre image.Image para no pagar una llamada a
// interfaz por pixel, que es lo que hacia lento el pipeline de imagen.
type canvas struct {
	pix  []uint8
	w, h int
}

func newCanvas(w, h int) *canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	c := &canvas{pix: make([]uint8, w*h), w: w, h: h}
	for i := range c.pix {
		c.pix[i] = 255 // papel en blanco
	}
	return c
}

func (c *canvas) set(x, y int, v uint8) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.pix[y*c.w+x] = v
}

func (c *canvas) at(x, y int) uint8 {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return 255
	}
	return c.pix[y*c.w+x]
}

// fill pinta un rectangulo solido.
func (c *canvas) fill(x, y, w, h int, v uint8) {
	for yy := y; yy < y+h; yy++ {
		if yy < 0 || yy >= c.h {
			continue
		}
		fila := c.pix[yy*c.w:]
		for xx := x; xx < x+w; xx++ {
			if xx < 0 || xx >= c.w {
				continue
			}
			fila[xx] = v
		}
	}
}

// rect dibuja el contorno de un rectangulo con el grosor indicado.
func (c *canvas) rect(x, y, w, h, grosor int) {
	if grosor < 1 {
		grosor = 1
	}
	c.fill(x, y, w, grosor, 0)
	c.fill(x, y+h-grosor, w, grosor, 0)
	c.fill(x, y, grosor, h, 0)
	c.fill(x+w-grosor, y, grosor, h, 0)
}

// invert cambia tinta por papel en un rectangulo, para el texto resaltado.
func (c *canvas) invert(x, y, w, h int) {
	for yy := y; yy < y+h; yy++ {
		if yy < 0 || yy >= c.h {
			continue
		}
		fila := c.pix[yy*c.w:]
		for xx := x; xx < x+w; xx++ {
			if xx < 0 || xx >= c.w {
				continue
			}
			fila[xx] = 255 - fila[xx]
		}
	}
}

// blit copia una imagen sobre el lienzo, quedandose con el pixel mas oscuro.
func (c *canvas) blit(src image.Image, x, y int) {
	b := src.Bounds()
	if g, ok := src.(*image.Gray); ok {
		for yy := 0; yy < b.Dy(); yy++ {
			off := (yy+b.Min.Y-g.Rect.Min.Y)*g.Stride + (b.Min.X - g.Rect.Min.X)
			fila := g.Pix[off : off+b.Dx()]
			for xx, v := range fila {
				if v < c.at(x+xx, y+yy) {
					c.set(x+xx, y+yy, v)
				}
			}
		}
		return
	}
	for yy := 0; yy < b.Dy(); yy++ {
		for xx := 0; xx < b.Dx(); xx++ {
			r, gg, bb, a := src.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
			if a == 0 {
				continue
			}
			l := uint8((299*uint32(r>>8) + 587*uint32(gg>>8) + 114*uint32(bb>>8)) / 1000)
			if l < c.at(x+xx, y+yy) {
				c.set(x+xx, y+yy, l)
			}
		}
	}
}

// gray devuelve el lienzo ya umbralizado a blanco y negro puro, que es lo que
// espera el empaquetado de la impresora.
func (c *canvas) gray() *image.Gray {
	out := image.NewGray(image.Rect(0, 0, c.w, c.h))
	for y := 0; y < c.h; y++ {
		src := c.pix[y*c.w : (y+1)*c.w]
		dst := out.Pix[y*out.Stride : y*out.Stride+c.w]
		for x, v := range src {
			if v < 128 {
				dst[x] = 0
			} else {
				dst[x] = 255
			}
		}
	}
	return out
}
