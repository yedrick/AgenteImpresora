// Vista previa de un bloque maquetado.

package api

import (
	"bytes"
	"image"
	"image/png"
	"net/http"

	"collatech-agent/internal/layout"
)

// maxPreviewScale acota el aumento de la vista previa.
const maxPreviewScale = 4

type previewRequest struct {
	Width int `json:"width"`
	Scale int `json:"scale"`
	// UpsideDown gira la previa 180 grados, igual que lo hara la impresora.
	// Sin esto, activar el giro en el disenador no cambiaba nada en
	// pantalla y solo se notaba con el papel en la mano.
	UpsideDown bool          `json:"upside_down"`
	Layout     layout.Layout `json:"layout"`
}

// preview devuelve el bloque compuesto como PNG, exactamente igual que se
// imprimiria. Es lo que hace que el disenador sea fiable: lo que se ve en
// pantalla no es una aproximacion, son los mismos puntos que salen por la
// impresora.
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if !s.decode(w, r, &req) {
		return
	}
	req.Layout.Width = printWidth(req.Width)
	img, err := layout.Render(req.Layout)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}
	if req.UpsideDown {
		img = girar180(img)
	}
	// El aumento es solo para la pantalla: a 203 ppp un ticket se ve
	// diminuto en un monitor.
	if req.Scale > 1 {
		if req.Scale > maxPreviewScale {
			req.Scale = maxPreviewScale
		}
		img = upscale(img, req.Scale)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: "no se pudo generar la vista previa"})
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(buf.Bytes())
}

// upscale repite cada pixel: asi se ve el punto real de la impresora en vez
// de una version suavizada que engane sobre como va a quedar.
func upscale(src image.Image, n int) image.Image {
	g, ok := src.(*image.Gray)
	if !ok {
		return src
	}
	b := g.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx()*n, b.Dy()*n))
	for y := 0; y < b.Dy(); y++ {
		fila := g.Pix[y*g.Stride : y*g.Stride+b.Dx()]
		for rep := 0; rep < n; rep++ {
			dst := out.Pix[(y*n+rep)*out.Stride:]
			for x, v := range fila {
				for dx := 0; dx < n; dx++ {
					dst[x*n+dx] = v
				}
			}
		}
	}
	return out
}

// girar180 devuelve la imagen boca abajo, que es lo que hace la impresora
// con el comando ESC { cuando se imprime girado.
func girar180(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewGray(image.Rect(0, 0, w, h))
	if g, ok := src.(*image.Gray); ok {
		for y := 0; y < h; y++ {
			fila := g.Pix[(b.Min.Y+y)*g.Stride+b.Min.X:][:w]
			destino := out.Pix[(h-1-y)*out.Stride:][:w]
			for x, v := range fila {
				destino[w-1-x] = v
			}
		}
		return out
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, gg, bb, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			l := uint8((299*uint32(r>>8) + 587*uint32(gg>>8) + 114*uint32(bb>>8)) / 1000)
			out.Pix[(h-1-y)*out.Stride+(w-1-x)] = l
		}
	}
	return out
}
