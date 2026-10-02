//go:build !windows

package printers

import "testing"

func TestLecturaDeLpstat(t *testing.T) {
	cases := []struct {
		line   string
		name   string
		online bool
		status string
	}{
		{"printer EPSON_TM_T20 is idle.  enabled since Thu 02 Oct 2026", "EPSON_TM_T20", true, "lista"},
		{"printer Caja disabled since Thu 02 Oct 2026 -", "Caja", false, "deshabilitada"},
		{"printer Cocina now printing Cocina-7.  enabled since hoy", "Cocina", true, "imprimiendo"},
	}
	for _, c := range cases {
		if got := lpstatName(c.line); got != c.name {
			t.Fatalf("nombre de %q: %q, esperaba %q", c.line, got, c.name)
		}
		online, status := parseLpstatLine(c.line)
		if online != c.online || status != c.status {
			t.Fatalf("estado de %q: (%v,%q), esperaba (%v,%q)", c.line, online, status, c.online, c.status)
		}
	}
}

func TestLpstatIgnoraLineasQueNoSonImpresoras(t *testing.T) {
	if got := lpstatName("no destinations added"); got != "" {
		t.Fatalf("esperaba cadena vacia, obtuve %q", got)
	}
}
