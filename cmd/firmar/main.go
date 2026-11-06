// Command firmar prepara las publicaciones para que el agente se pueda
// actualizar desde el panel.
//
//	firmar generar <carpeta>                  pareja de claves nueva (una vez)
//	firmar manifiesto -dir dist -version X    manifiesto.json con las sumas
//	firmar firmar <archivo>                   <archivo>.sig con FIRMA_ACTUALIZACIONES
//
// La clave privada nunca entra en el repositorio: vive en el secreto
// FIRMA_ACTUALIZACIONES de GitHub Actions y en la copia que guarda el dueno.
// Ver docs/ACTUALIZAR.md.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"collatech-agent/internal/actualizar"
)

const prefijo = "actualizacion-"

func main() {
	if len(os.Args) < 2 {
		uso()
	}
	var err error
	switch os.Args[1] {
	case "generar":
		if len(os.Args) != 3 {
			uso()
		}
		err = generarClaves(os.Args[2])
		if err == nil {
			fmt.Printf("Claves en %s. Lee docs/ACTUALIZAR.md antes de hacer nada con ellas.\n", os.Args[2])
		}
	case "manifiesto":
		fs := flag.NewFlagSet("manifiesto", flag.ExitOnError)
		dir := fs.String("dir", "dist", "carpeta con los actualizacion-*")
		version := fs.String("version", "", "version que se publica")
		minima := fs.String("minima", "", "version minima desde la que vale el boton")
		manual := fs.String("manual", "", "si se pone, esta version se instala a mano y este texto dice por que")
		fs.Parse(os.Args[2:])
		err = escribirManifiesto(*dir, *version, *minima, *manual)
	case "firmar":
		if len(os.Args) != 3 {
			uso()
		}
		// FIRMA_CLAVE_DE_PRUEBA=1 solo lo pone scripts/prueba-actualizar.sh,
		// que firma con una clave de prueba a proposito.
		err = firmarArchivo(os.Args[2], os.Getenv("FIRMA_ACTUALIZACIONES"), os.Getenv("FIRMA_CLAVE_DE_PRUEBA") == "1")
	default:
		uso()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func uso() {
	fmt.Fprintln(os.Stderr, "uso: firmar generar <carpeta> | manifiesto -dir D -version X | firmar <archivo>")
	os.Exit(2)
}

// generarClaves escribe privada.txt (semilla de 32 bytes en base64, 0600) y
// publica.txt. Se niega a pisar una clave existente y a escribir dentro de
// un repositorio, donde acabaria subida por descuido.
func generarClaves(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if repo := dentroDeRepositorio(abs); repo != "" {
		return fmt.Errorf("%s esta dentro del repositorio %s: la clave privada acabaria subida. Usa una carpeta fuera, por ejemplo ~/.collatech-firma", abs, repo)
	}
	privada := filepath.Join(abs, "privada.txt")
	if _, err := os.Stat(privada); err == nil {
		return fmt.Errorf("ya hay una clave en %s: no la piso", privada)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return err
	}
	semilla := base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"
	if err := os.WriteFile(privada, []byte(semilla), 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(abs, "publica.txt"), []byte(base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644)
}

func dentroDeRepositorio(dir string) string {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// escribirManifiesto lee los actualizacion-* de dir y escribe
// dir/manifiesto.json con la suma de cada uno, por plataforma.
func escribirManifiesto(dir, version, minima, manual string) error {
	if strings.TrimSpace(version) == "" {
		return errors.New("falta -version")
	}
	entradas, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	m := actualizar.Manifiesto{
		Version:  strings.TrimPrefix(version, "v"),
		Minima:   minima,
		Manual:   manual,
		Archivos: map[string]actualizar.Archivo{},
	}
	for _, e := range entradas {
		nombre := e.Name()
		if e.IsDir() || !strings.HasPrefix(nombre, prefijo) {
			continue
		}
		plataforma := strings.TrimSuffix(strings.TrimPrefix(nombre, prefijo), ".exe")
		suma, err := sumaDe(filepath.Join(dir, nombre))
		if err != nil {
			return err
		}
		m.Archivos[plataforma] = actualizar.Archivo{Nombre: nombre, SHA256: suma}
	}
	if len(m.Archivos) == 0 {
		return fmt.Errorf("no hay ningun %s* en %s", prefijo, dir)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	plataformas := make([]string, 0, len(m.Archivos))
	for p := range m.Archivos {
		plataformas = append(plataformas, p)
	}
	sort.Strings(plataformas)
	fmt.Printf("manifiesto.json %s: %s\n", m.Version, strings.Join(plataformas, ", "))
	return os.WriteFile(filepath.Join(dir, "manifiesto.json"), append(b, '\n'), 0o644)
}

func sumaDe(ruta string) (string, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// firmarArchivo firma los bytes exactos de ruta con la semilla en base64 y
// escribe ruta.sig. Antes comprueba que la clave sea una de las oficiales
// (salvo permitirNoOficial, para la prueba de CI) y que la firma verifique:
// asi nunca se publica una firma que las cajas rechazarian.
func firmarArchivo(ruta, semillaB64 string, permitirNoOficial bool) error {
	if strings.TrimSpace(semillaB64) == "" {
		return errors.New("falta el secreto FIRMA_ACTUALIZACIONES: sin el, el boton de actualizar no podria instalar esta version. Ver docs/ACTUALIZAR.md")
	}
	semilla, err := base64.StdEncoding.DecodeString(strings.TrimSpace(semillaB64))
	if err != nil || len(semilla) != ed25519.SeedSize {
		return errors.New("FIRMA_ACTUALIZACIONES no es una clave valida: tiene que ser el contenido de privada.txt tal cual")
	}
	priv := ed25519.NewKeyFromSeed(semilla)
	pub := priv.Public().(ed25519.PublicKey)
	if !permitirNoOficial && !esOficial(pub) {
		return fmt.Errorf("la clave de FIRMA_ACTUALIZACIONES (publica %s) no es ninguna de las oficiales de internal/actualizar/claves.go: las cajas rechazarian esta version. Revisa el secreto (docs/ACTUALIZAR.md)",
			base64.StdEncoding.EncodeToString(pub))
	}
	datos, err := os.ReadFile(ruta)
	if err != nil {
		return err
	}
	firma := ed25519.Sign(priv, datos)
	if !ed25519.Verify(priv.Public().(ed25519.PublicKey), datos, firma) {
		return errors.New("la firma recien hecha no verifica")
	}
	publica := base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))
	fmt.Printf("firmado %s con la clave publica %s\n", filepath.Base(ruta), publica)
	return os.WriteFile(ruta+".sig", []byte(base64.StdEncoding.EncodeToString(firma)+"\n"), 0o644)
}

func esOficial(pub ed25519.PublicKey) bool {
	for _, c := range actualizar.ClavesOficiales() {
		if c.Equal(pub) {
			return true
		}
	}
	return false
}
