package layout

import (
	"sync"
	"testing"
)

// El agente compone bloques desde varios workers de la cola y desde las
// peticiones de vista previa a la vez.
func TestRenderConcurrente(t *testing.T) {
	l := Layout{Width: 576, Rows: []Row{
		{Cols: []Col{
			{Weight: 1, Items: []Item{{Text: "Factura #F-000123 para Ana Peña", Size: "l", Bold: true}}},
			{Dots: 140, Items: []Item{{QR: "https://ejemplo.com/f/123"}}},
		}},
	}}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := Render(l); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
