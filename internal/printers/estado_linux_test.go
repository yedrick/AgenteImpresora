//go:build linux

package printers

import "testing"

func TestInterpretarLP(t *testing.T) {
	casos := []struct {
		st    int
		ready bool
		paper string
	}{
		{lpPSelecd | lpPError, true, PaperOK},
		{lpPSelecd | lpPError | lpPOutpa, false, PaperOut},
		{lpPError, false, PaperOK},  // sin SELECT: fuera de linea
		{lpPSelecd, false, PaperOK}, // ERROR a cero: error
	}
	for _, c := range casos {
		e := interpretarLP(c.st).cerrar()
		if e.Ready != c.ready || e.Paper != c.paper {
			t.Errorf("estado 0x%02x: ready=%v paper=%s, se esperaba ready=%v paper=%s", c.st, e.Ready, e.Paper, c.ready, c.paper)
		}
	}
}
