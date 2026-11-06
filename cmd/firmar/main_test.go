package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"collatech-agent/internal/actualizar"
)

func TestManifiestoDesdeCarpeta(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "actualizacion-linux-amd64"), []byte("linux"), 0o755)
	os.WriteFile(filepath.Join(dir, "actualizacion-windows-386.exe"), []byte("windows"), 0o755)
	os.WriteFile(filepath.Join(dir, "otra-cosa.zip"), []byte("no cuenta"), 0o644)

	if err := escribirManifiesto(dir, "1.7.0", "", ""); err != nil {
		t.Fatal(err)
	}
	var m actualizar.Manifiesto
	b, _ := os.ReadFile(filepath.Join(dir, "manifiesto.json"))
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != "1.7.0" || len(m.Archivos) != 2 {
		t.Fatalf("manifiesto inesperado: %+v", m)
	}
	suma := sha256.Sum256([]byte("windows"))
	if a := m.Archivos["windows-386"]; a.Nombre != "actualizacion-windows-386.exe" || a.SHA256 != hex.EncodeToString(suma[:]) {
		t.Fatalf("entrada de windows-386 mal: %+v", a)
	}
	if _, ok := m.Archivos["linux-amd64"]; !ok {
		t.Fatal("falta linux-amd64")
	}
}

func TestManifiestoSinEjecutables(t *testing.T) {
	if err := escribirManifiesto(t.TempDir(), "1.7.0", "", ""); err == nil {
		t.Fatal("un manifiesto sin ejecutables no sirve para actualizar: tiene que dar error")
	}
}

func TestFirmarSinSecreto(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "manifiesto.json")
	os.WriteFile(ruta, []byte(`{"version":"1.7.0"}`), 0o644)
	err := firmarArchivo(ruta, "", false)
	if err == nil || !strings.Contains(err.Error(), "FIRMA_ACTUALIZACIONES") {
		t.Fatalf("quiero un error que nombre FIRMA_ACTUALIZACIONES, tengo %v", err)
	}
}

func TestFirmarYVerificar(t *testing.T) {
	dir := t.TempDir()
	if err := generarClaves(dir); err != nil {
		t.Fatal(err)
	}
	priv, _ := os.ReadFile(filepath.Join(dir, "privada.txt"))
	pubTxt, _ := os.ReadFile(filepath.Join(dir, "publica.txt"))
	if info, _ := os.Stat(filepath.Join(dir, "privada.txt")); info.Mode().Perm() != 0o600 {
		t.Fatalf("la privada tiene que quedar con permisos 0600, tiene %v", info.Mode().Perm())
	}

	ruta := filepath.Join(dir, "manifiesto.json")
	datos := []byte(`{"version":"1.7.0","archivos":{}}`)
	os.WriteFile(ruta, datos, 0o644)
	if err := firmarArchivo(ruta, string(priv), true); err != nil {
		t.Fatal(err)
	}
	firma, err := os.ReadFile(ruta + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pubTxt)))
	if _, err := actualizar.VerificarManifiesto(datos, firma, []ed25519.PublicKey{pub}); err != nil {
		t.Fatalf("la firma generada no verifica: %v", err)
	}
}

func TestGenerarNoPisaClaves(t *testing.T) {
	dir := t.TempDir()
	if err := generarClaves(dir); err != nil {
		t.Fatal(err)
	}
	if err := generarClaves(dir); err == nil {
		t.Fatal("generar otra vez en la misma carpeta pisaria la clave privada: tiene que negarse")
	}
}

// Si el secreto de GitHub tiene una clave que no es la oficial, todas las
// cajas rechazarian la version: mejor que la publicacion falle.
func TestFirmarConClaveNoOficial(t *testing.T) {
	dir := t.TempDir()
	if err := generarClaves(dir); err != nil {
		t.Fatal(err)
	}
	priv, _ := os.ReadFile(filepath.Join(dir, "privada.txt"))
	ruta := filepath.Join(dir, "manifiesto.json")
	os.WriteFile(ruta, []byte(`{"version":"1.7.0","archivos":{}}`), 0o644)
	err := firmarArchivo(ruta, string(priv), false)
	if err == nil || !strings.Contains(err.Error(), "oficiales") {
		t.Fatalf("firmar con una clave que no es oficial tiene que fallar, tengo %v", err)
	}
	if _, err := os.Stat(ruta + ".sig"); err == nil {
		t.Fatal("no puede quedar una firma que las cajas rechazarian")
	}
}
