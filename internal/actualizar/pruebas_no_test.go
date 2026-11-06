//go:build !actualizarprueba

package actualizar

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// Los binarios publicados no pueden apuntar a otra publicacion ni aceptar
// otra clave, aunque alguien ponga las variables de la prueba de CI.
func TestPublicadoIgnoraLasVariablesDePrueba(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv("COLLATECH_CLAVE_PRUEBA", base64.StdEncoding.EncodeToString(pub))
	t.Setenv("COLLATECH_URL_ACTUALIZACIONES", "http://127.0.0.1:1/latest")
	if n := len(ClavesDelAgente()); n != 2 {
		t.Fatalf("un binario publicado solo acepta las 2 claves oficiales, acepta %d", n)
	}
	if u := FuenteOficial("1.0.0").URLUltima; u != URLUltimaOficial {
		t.Fatalf("un binario publicado solo consulta %s, consulta %s", URLUltimaOficial, u)
	}
}
