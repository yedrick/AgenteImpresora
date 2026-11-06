package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"collatech-agent/internal/config"
)

func TestHealthIncluyeVersion(t *testing.T) {
	h := newTestServer(t, nil)
	rec := get(t, h, "/health", "", "")
	var r struct {
		OK   bool `json:"ok"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	// newTestServer no fija Version: "dev" es lo que llevan las compilaciones
	// sin sellar, y el campo tiene que ir siempre.
	if !r.OK || r.Data.Version == "" {
		t.Fatalf("/health tiene que decir la version: %s", rec.Body.String())
	}
}

func TestActualizacionSoloLocal(t *testing.T) {
	h := newTestServer(t, func(c *config.Config) {
		c.AllowRemote = true
		c.AuthToken = "secreto"
	})
	for _, ruta := range []string{"/api/actualizacion"} {
		if rec := get(t, h, ruta, "192.168.1.50:5000", "secreto"); rec.Code != http.StatusForbidden {
			t.Fatalf("GET %s desde la red con token: %d, quiero 403", ruta, rec.Code)
		}
	}
	for _, ruta := range []string{"/api/actualizacion/buscar", "/api/actualizacion/instalar"} {
		req := post(t, h, ruta, `{"version":"9.9.9"}`, "192.168.1.50:5000")
		if req.Code != http.StatusForbidden {
			t.Fatalf("POST %s desde la red: %d, quiero 403", ruta, req.Code)
		}
	}
}

func TestActualizacionSinActualizador(t *testing.T) {
	h := newTestServer(t, nil)
	rec := get(t, h, "/api/actualizacion", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/actualizacion: %d %s", rec.Code, rec.Body.String())
	}
	var r struct {
		Data struct {
			Soportado bool   `json:"soportado"`
			Motivo    string `json:"motivo"`
		} `json:"data"`
	}
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Data.Soportado || r.Data.Motivo == "" {
		t.Fatalf("sin actualizador: soportado=false con motivo. %s", rec.Body.String())
	}
	if rec := post(t, h, "/api/actualizacion/instalar", `{"version":"9.9.9"}`, ""); rec.Code == http.StatusAccepted {
		t.Fatal("sin actualizador no se puede instalar")
	}
	if rec := post(t, h, "/api/actualizacion/buscar", `{}`, ""); rec.Code == http.StatusOK {
		t.Fatal("sin actualizador no se puede buscar")
	}
}
