package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNingunConfigLlevaToken comprueba que ningun config.json del arbol
// guarda un token de acceso.
//
// Hace falta porque configs/config.json esta versionado y el agente le
// escribe un token generado la primera vez que arranca. Basta con probar el
// agente desde el propio directorio del proyecto, que es justo lo que hace
// ABRIR_PANEL_COLLATECH.bat, para dejar un secreto listo para subir. Y si se
// colara en ENTREGA_CLIENTE, todos los clientes recibirian el mismo token.
func TestNingunConfigLlevaToken(t *testing.T) {
	raiz := filepath.Join("..", "..")
	revisados := 0

	err := filepath.Walk(raiz, func(ruta string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // un directorio ilegible no es asunto de esta prueba
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "dist", "prueba":
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() != "config.json" {
			return nil
		}
		datos, err := os.ReadFile(ruta)
		if err != nil {
			return nil
		}
		var c struct {
			AuthToken string `json:"auth_token"`
		}
		if json.Unmarshal(datos, &c) != nil {
			return nil // no es un config nuestro
		}
		revisados++
		if strings.TrimSpace(c.AuthToken) != "" {
			rel, _ := filepath.Rel(raiz, ruta)
			t.Errorf("%s lleva un token dentro.\n"+
				"Dejalo en \"\" antes de subir: el agente genera uno nuevo al arrancar.", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if revisados == 0 {
		t.Fatal("no se reviso ningun config.json: la prueba no comprueba nada")
	}
	t.Logf("%d archivos config.json revisados, ninguno con token", revisados)
}
