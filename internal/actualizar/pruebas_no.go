//go:build !actualizarprueba

package actualizar

import "crypto/ed25519"

// En los binarios que se publican no hay forma de cambiar de donde se
// actualiza ni de anadir claves: ver pruebas.go.
func clavesDePrueba() []ed25519.PublicKey { return nil }

func urlDePrueba() string { return "" }

func systemctlUsuario() bool { return false }
