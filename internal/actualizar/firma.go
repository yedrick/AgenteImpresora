package actualizar

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Archivo es un ejecutable de la publicacion, con la suma que debe tener.
type Archivo struct {
	Nombre string `json:"nombre"`
	SHA256 string `json:"sha256"`
}

// Manifiesto describe una publicacion. Se firma entero: la firma cubre los
// bytes exactos del archivo, asi que nadie puede cambiar una suma ni una
// version sin invalidarla.
type Manifiesto struct {
	Version string `json:"version"`
	// Minima es la version minima desde la que se puede actualizar con el
	// boton. Por debajo, el panel dice que se instale a mano.
	Minima string `json:"minima,omitempty"`
	// Manual, si trae texto, dice que esta version se instala a mano (por
	// ejemplo porque cambia unidades de systemd) y explica por que.
	Manual string `json:"manual,omitempty"`
	// Archivos va por plataforma: "linux-amd64", "windows-386"...
	Archivos map[string]Archivo `json:"archivos"`
}

// VerificarManifiesto comprueba la firma (base64, Ed25519) con cualquiera de
// las claves y solo entonces lee el manifiesto. Los campos que no conoce se
// ignoran, para que versiones futuras puedan anadir mas.
func VerificarManifiesto(datos, firma []byte, claves []ed25519.PublicKey) (Manifiesto, error) {
	var m Manifiesto
	if len(claves) == 0 {
		return m, errors.New("el agente no tiene claves para verificar la firma")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(firma)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return m, errors.New("la firma del manifiesto no tiene un formato valido")
	}
	valida := false
	for _, c := range claves {
		if len(c) == ed25519.PublicKeySize && ed25519.Verify(c, datos, sig) {
			valida = true
			break
		}
	}
	if !valida {
		return m, errors.New("la firma del manifiesto no es valida: esta version no salio de las publicaciones oficiales")
	}
	if err := json.Unmarshal(datos, &m); err != nil {
		return m, fmt.Errorf("el manifiesto firmado no se puede leer: %w", err)
	}
	if strings.TrimSpace(m.Version) == "" {
		return m, errors.New("el manifiesto no dice que version es")
	}
	return m, nil
}

// VerificarArchivo comprueba que el archivo tenga la suma SHA256 esperada.
func VerificarArchivo(ruta, sha256hex string) error {
	f, err := os.Open(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, strings.TrimSpace(sha256hex)) {
		return fmt.Errorf("el archivo descargado no coincide con el de la publicacion (puede que la descarga se cortara): esperado %s, tiene %s", sha256hex, got)
	}
	return nil
}

// ClavesOficiales devuelve las claves publicas con las que se firman las
// publicaciones. Las que no decodifiquen a una clave Ed25519 se descartan.
func ClavesOficiales() []ed25519.PublicKey {
	var out []ed25519.PublicKey
	for _, s := range clavesOficiales {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err == nil && len(b) == ed25519.PublicKeySize {
			out = append(out, ed25519.PublicKey(b))
		}
	}
	return out
}
