package escpos

import (
	"image"
	"image/color"
	"math/rand"
	"strings"
	"testing"
)

func logoDePrueba(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := uint8(rnd.Intn(256))
			img.Set(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return img
}

// Un logo de 600x400 escalado a 576 puntos: el caso tipico de
// POST /api/print/logo y del logo de un ticket.
func BenchmarkImageFit(b *testing.B) {
	img := logoDePrueba(600, 400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New().ImageFit(img, 576).Bytes()
	}
}

func BenchmarkPrepareImage(b *testing.B) {
	img := logoDePrueba(600, 400)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = PrepareImage(img, 576)
	}
}

// Empaquetado a 1 bit por pixel de una imagen ya preparada.
func BenchmarkImageRaster(b *testing.B) {
	img := PrepareImage(logoDePrueba(600, 400), 576)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New().Image(img).Bytes()
	}
}

func BenchmarkEncodeCP850(b *testing.B) {
	texto := strings.Repeat("Cafe con leche y azucar - Bs 12.00\n", 40)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = EncodeCP850(texto)
	}
}
