package actualizar

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clavePrueba(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func firmar(priv ed25519.PrivateKey, datos []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, datos)) + "\n")
}

const manifiestoPrueba = `{"version":"1.7.0","archivos":{"linux-amd64":{"nombre":"actualizacion-linux-amd64","sha256":"abc"}}}`

func TestManifiestoFirmaValida(t *testing.T) {
	pub, priv := clavePrueba(t)
	datos := []byte(manifiestoPrueba)
	m, err := VerificarManifiesto(datos, firmar(priv, datos), []ed25519.PublicKey{pub})
	if err != nil {
		t.Fatalf("firma valida rechazada: %v", err)
	}
	if m.Version != "1.7.0" || m.Archivos["linux-amd64"].Nombre != "actualizacion-linux-amd64" {
		t.Fatalf("manifiesto mal leido: %+v", m)
	}
}

func TestManifiestoFirmaDeRespaldo(t *testing.T) {
	pub1, _ := clavePrueba(t)
	pub2, priv2 := clavePrueba(t)
	datos := []byte(manifiestoPrueba)
	if _, err := VerificarManifiesto(datos, firmar(priv2, datos), []ed25519.PublicKey{pub1, pub2}); err != nil {
		t.Fatalf("la firma con la clave de respaldo tiene que valer: %v", err)
	}
}

func TestManifiestoFirmaInvalida(t *testing.T) {
	pub, priv := clavePrueba(t)
	datos := []byte(manifiestoPrueba)
	firma := firmar(priv, datos)
	cambiado := []byte(strings.Replace(manifiestoPrueba, "1.7.0", "1.7.1", 1))
	if _, err := VerificarManifiesto(cambiado, firma, []ed25519.PublicKey{pub}); err == nil {
		t.Fatal("un manifiesto cambiado no puede pasar la firma")
	}
	otra, _ := clavePrueba(t)
	if _, err := VerificarManifiesto(datos, firma, []ed25519.PublicKey{otra}); err == nil {
		t.Fatal("una firma de otra clave no puede valer")
	}
	if _, err := VerificarManifiesto(datos, firma, nil); err == nil {
		t.Fatal("sin claves no se puede aceptar nada")
	}
}

func TestManifiestoCampoDesconocido(t *testing.T) {
	pub, priv := clavePrueba(t)
	datos := []byte(`{"version":"1.7.0","nuevo":1,"archivos":{}}`)
	if _, err := VerificarManifiesto(datos, firmar(priv, datos), []ed25519.PublicKey{pub}); err != nil {
		t.Fatalf("un campo desconocido no debe romper la lectura: %v", err)
	}
}

func TestVerificarArchivo(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "binario")
	contenido := []byte("contenido del binario")
	if err := os.WriteFile(ruta, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	suma := sha256.Sum256(contenido)
	if err := VerificarArchivo(ruta, hex.EncodeToString(suma[:])); err != nil {
		t.Fatalf("suma correcta rechazada: %v", err)
	}
}

func TestVerificarArchivoNoCoincide(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "binario")
	if err := os.WriteFile(ruta, []byte("otro contenido"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := VerificarArchivo(ruta, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "no coincide") {
		t.Fatalf("quiero un error que diga \"no coincide\", tengo %v", err)
	}
}

func TestClavesOficialesSonValidas(t *testing.T) {
	for i, c := range ClavesOficiales() {
		if len(c) != ed25519.PublicKeySize {
			t.Fatalf("clave oficial %d con tamano %d", i, len(c))
		}
	}
}
