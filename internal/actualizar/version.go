// Package actualizar busca, verifica e instala versiones nuevas del agente
// desde el panel. Solo actua cuando alguien pulsa el boton: nunca consulta
// nada por su cuenta.
package actualizar

import (
	"strconv"
	"strings"
)

// Comparar devuelve -1, 0 o 1 segun a sea menor, igual o mayor que b.
//
// Acepta "1.6.3" y "v1.6.3". Una version que no es numerica ("dev",
// "ci-abc1234": compilaciones de prueba) es menor que cualquier numerica,
// asi que a una compilacion de prueba siempre se le ofrece la publicada.
func Comparar(a, b string) int {
	na, okA := partes(a)
	nb, okB := partes(b)
	switch {
	case !okA && !okB:
		return 0
	case !okA:
		return -1
	case !okB:
		return 1
	}
	for i := 0; i < 3; i++ {
		if na[i] != nb[i] {
			if na[i] < nb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// EsMayor dice si nueva es estrictamente mayor que actual.
func EsMayor(nueva, actual string) bool {
	return Comparar(nueva, actual) > 0
}

// partes lee mayor.menor.parche. Lo que venga tras un "-" o "+" (una
// etiqueta de prueba) no cuenta.
func partes(v string) ([3]int, bool) {
	var n [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	trozos := strings.Split(v, ".")
	if len(trozos) == 0 || len(trozos) > 3 {
		return n, false
	}
	for i, t := range trozos {
		x, err := strconv.Atoi(t)
		if err != nil || x < 0 {
			return n, false
		}
		n[i] = x
	}
	return n, true
}
