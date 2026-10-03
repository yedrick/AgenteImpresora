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
	Width  int           `json:"width"`
	Scale  int           `json:"scale"`
	Layout layout.Layout `json:"layout"`
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
