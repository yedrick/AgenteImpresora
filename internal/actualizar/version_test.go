package actualizar

import "testing"

func TestComparar(t *testing.T) {
	casos := []struct {
		a, b string
		want int
	}{
		{"1.6.3", "1.6.10", -1},
		{"v1.7.0", "1.6.9", 1},
		{"1.6.3", "1.6.3", 0},
		{"v1.6.3", "1.6.3", 0},
		{"dev", "0.0.1", -1},
		{"0.0.1", "ci-abc1234", 1},
		{"dev", "dev", 0},
		{"2.0.0", "1.99.99", 1},
	}
	for _, c := range casos {
		if got := Comparar(c.a, c.b); got != c.want {
			t.Errorf("Comparar(%q, %q) = %d, quiero %d", c.a, c.b, got, c.want)
		}
	}
}

func TestEsMayor(t *testing.T) {
	if !EsMayor("1.6.4", "1.6.3") {
		t.Error("1.6.4 tiene que ser mayor que 1.6.3")
	}
	if EsMayor("1.6.3", "1.6.3") {
		t.Error("la misma version no es mayor")
	}
	if EsMayor("1.6.2", "1.6.3") {
		t.Error("una version menor no es mayor")
	}
}
