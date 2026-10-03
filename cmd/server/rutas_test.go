package main

import (
	"path/filepath"
	"testing"
)

// TestRutasRelativasDesdeDondeSeLlama: --config y --data-dir se resuelven
// contra el directorio desde el que se invoco el agente.
//
// Antes se resolvian despues del Chdir al directorio del ejecutable, asi que
// "--data-dir ." no apuntaba a la carpeta actual sino a la del binario. El
// agente leia y escribia en otro sitio sin avisar: probar con una carpeta
// aparte tocaba la configuracion de verdad.
func TestRutasRelativasDesdeDondeSeLlama(t *testing.T) {
	casos := []struct {
		nombre, base, ruta, quiero string
	}{
		{"punto", "/home/ana/prueba", ".", "/home/ana/prueba"},
		{"relativa", "/home/ana/prueba", "configs/config.json", "/home/ana/prueba/configs/config.json"},
		{"hacia arriba", "/home/ana/prueba", "../datos", "/home/ana/datos"},
		{"ya absoluta", "/home/ana/prueba", "/etc/collatech/config.json", "/etc/collatech/config.json"},
		{"vacia se respeta", "/home/ana/prueba", "", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := desde(c.base, c.ruta); got != filepath.Clean(c.quiero) && got != c.quiero {
				t.Errorf("desde(%q, %q) = %q, se esperaba %q", c.base, c.ruta, got, c.quiero)
			}
		})
	}
}
