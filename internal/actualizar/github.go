package actualizar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// URLUltimaOficial es la ultima publicacion del repositorio del agente.
const URLUltimaOficial = "https://api.github.com/repos/yedrick/AgenteImpresora/releases/latest"

// Publicacion es lo que interesa de una Release de GitHub.
type Publicacion struct {
	Version  string
	URLNotas string
	// Recursos va del nombre del archivo a su URL de descarga.
	Recursos map[string]string
}

// Fuente consulta y descarga las publicaciones.
type Fuente struct {
	URLUltima string
	Cliente   *http.Client
	UserAgent string
}

// FuenteOficial apunta a las publicaciones de GitHub. El User-Agent solo
// lleva el nombre y la version del agente: nada del usuario ni del equipo.
func FuenteOficial(version string) Fuente {
	url := URLUltimaOficial
	if u := urlDePrueba(); u != "" {
		url = u
	}
	return Fuente{
		URLUltima: url,
		Cliente:   &http.Client{Timeout: 15 * time.Second},
		UserAgent: "CollaTech-Agent/" + version,
	}
}

// Ultima devuelve la ultima publicacion.
func (f Fuente) Ultima(ctx context.Context) (Publicacion, error) {
	var p Publicacion
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URLUltima, nil)
	if err != nil {
		return p, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	f.firmarPeticion(req)
	res, err := f.cliente(30 * time.Second).Do(req)
	if err != nil {
		return p, fmt.Errorf("No se pudo consultar GitHub (¿hay internet en esta PC?): %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		cuerpo, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return p, fmt.Errorf("No se pudo consultar GitHub: respondio %d %s", res.StatusCode, strings.TrimSpace(string(cuerpo)))
	}
	var r struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&r); err != nil {
		return p, fmt.Errorf("No se pudo leer la respuesta de GitHub: %w", err)
	}
	if r.TagName == "" {
		return p, errors.New("No se pudo consultar GitHub: la respuesta no trae version")
	}
	p.Version = strings.TrimPrefix(r.TagName, "v")
	p.URLNotas = r.HTMLURL
	p.Recursos = map[string]string{}
	for _, a := range r.Assets {
		p.Recursos[a.Name] = a.URL
	}
	return p, nil
}

// Descargar baja url a destino. Escribe en destino.parcial y renombra al
// terminar, asi que nunca queda un archivo a medias con el nombre bueno. Si
// pasa de max bytes, lo corta y lo borra.
func (f Fuente) Descargar(ctx context.Context, url, destino string, max int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	f.firmarPeticion(req)
	res, err := f.cliente(10 * time.Minute).Do(req)
	if err != nil {
		return fmt.Errorf("no se pudo descargar %s: %w", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("no se pudo descargar %s: respondio %d", url, res.StatusCode)
	}
	parcial := destino + ".parcial"
	out, err := os.OpenFile(parcial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(res.Body, max+1))
	cerrar := out.Close()
	if err == nil && n > max {
		err = fmt.Errorf("la descarga de %s pasa de %d bytes: se corta", url, max)
	}
	if err == nil {
		err = cerrar
	}
	if err != nil {
		os.Remove(parcial)
		return err
	}
	return os.Rename(parcial, destino)
}

func (f Fuente) firmarPeticion(req *http.Request) {
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
}

func (f Fuente) cliente(def time.Duration) *http.Client {
	if f.Cliente != nil {
		return f.Cliente
	}
	return &http.Client{Timeout: def}
}
