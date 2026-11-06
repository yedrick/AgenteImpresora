//go:build actualizarprueba

package actualizar

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"strings"
)

// Solo existe en compilaciones con la etiqueta actualizarprueba, que usa la
// prueba de punta a punta de CI: apunta el boton a una publicacion falsa
// firmada con una clave de prueba. Los binarios publicados no leen estas
// variables.

func clavesDePrueba() []ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("COLLATECH_CLAVE_PRUEBA")))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return []ed25519.PublicKey{ed25519.PublicKey(b)}
}

func urlDePrueba() string { return os.Getenv("COLLATECH_URL_ACTUALIZACIONES") }

// systemctlUsuario hace que la prueba local use unidades de usuario
// (systemctl --user), para probar el flujo entero sin ser root.
func systemctlUsuario() bool { return os.Getenv("COLLATECH_SYSTEMCTL_USUARIO") == "1" }
