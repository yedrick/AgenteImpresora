//go:build linux

package printers

import (
	"io"
	"os"
	"testing"
	"time"
)

// tuberiaLlena devuelve una tuberia cuyo buffer no admite ni un byte mas. Es
// lo mas parecido a una impresora USB que aun no termino de recibir: POLLOUT
// apagado hasta que el otro lado consuma.
func tuberiaLlena(t *testing.T) (r, w *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	_ = w.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := w.Write(make([]byte, 1<<20)); err == nil {
		t.Fatal("la tuberia acepto 1 MB: no quedo llena")
	}
	_ = w.SetWriteDeadline(time.Time{})
	return r, w
}

// TestEsperaAQueSalgaTodo: Print no puede volver mientras el destino tenga
// datos pendientes. Antes volvia enseguida, el cierre de despues cancelaba el
// envio en usblp y la impresora no recibia nada.
func TestEsperaAQueSalgaTodo(t *testing.T) {
	r, w := tuberiaLlena(t)

	const lector = 150 * time.Millisecond
	go func() {
		time.Sleep(lector)
		_, _ = io.Copy(io.Discard, r)
	}()

	inicio := time.Now()
	if err := esperarEnvio(w); err != nil {
		t.Fatalf("la espera fallo: %v", err)
	}
	if transcurrido := time.Since(inicio); transcurrido < lector {
		t.Errorf("volvio a los %v, antes de que el destino recibiera los datos", transcurrido)
	}
}

// TestEsperaConDeadline: una impresora que no acepta datos tiene que acabar
// en error, no dejar colgado al worker.
func TestEsperaConDeadline(t *testing.T) {
	_, w := tuberiaLlena(t)
	_ = w.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))

	hecho := make(chan error, 1)
	go func() { hecho <- esperarEnvio(w) }()
	select {
	case err := <-hecho:
		if err == nil {
			t.Fatal("dio por enviado un trabajo que el destino nunca recibio")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la espera ignoro el deadline")
	}
}
