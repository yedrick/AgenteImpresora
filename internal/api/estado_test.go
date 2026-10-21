package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Una estacion que imprime en un archivo y otra en una IP donde no hay nadie:
// /api/stations tiene que decir que la primera esta lista y la segunda no, y
// por que.
func TestEstadoDeLasEstaciones(t *testing.T) {
	h := newTestServer(t, nil)

	if err := os.WriteFile("rollo.bin", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	apagada := ln.Addr().String()
	ln.Close()

	alta := `[{"name":"caja","target":"device://` + filepath.Join(mustWd(t), "rollo.bin") + `"},` +
		`{"name":"cocina","target":"tcp://` + apagada + `"}]`
	if rec := post(t, h, "/api/printers-config", alta, ""); rec.Code != http.StatusOK {
		t.Fatalf("alta: %d %s", rec.Code, rec.Body)
	}

	rec := get(t, h, "/api/stations", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/stations: %d %s", rec.Code, rec.Body)
	}
	var resp struct {
		Data []EstadoEstacion `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("esperaba 2 estaciones: %s", rec.Body)
	}
	caja, cocina := resp.Data[0], resp.Data[1]
	if !caja.Ready || !caja.Online {
		t.Errorf("caja deberia estar lista: %+v", caja)
	}
	if cocina.Online || cocina.Ready || !strings.Contains(cocina.Targets[0].Error, "no responde") {
		t.Errorf("cocina apunta a un puerto cerrado y deberia salir fuera de linea con el motivo: %+v", cocina)
	}

	// Una sola, por nombre.
	rec = get(t, h, "/api/stations/caja", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ready":true`) {
		t.Fatalf("GET /api/stations/caja: %d %s", rec.Code, rec.Body)
	}
}

// Quien imprime recibe un id y tiene que poder preguntar por el.
func TestConsultarUnTrabajo(t *testing.T) {
	h := newTestServer(t, nil)
	if err := os.WriteFile("rollo.bin", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	destino := "device://" + filepath.Join(mustWd(t), "rollo.bin")
	rec := post(t, h, "/api/print/text", `{"printer":"`+destino+`","text":"hola"}`, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("imprimir: %d %s", rec.Code, rec.Body)
	}
	var enc struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &enc); err != nil || enc.Data.ID == "" {
		t.Fatalf("sin id en la respuesta: %s", rec.Body)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		rec = get(t, h, "/api/jobs/"+enc.Data.ID, "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/jobs: %d %s", rec.Code, rec.Body)
		}
		if strings.Contains(rec.Body.String(), `"Completed"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("el trabajo no termino: %s", rec.Body)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Contains(rec.Body.String(), "payload") {
		t.Errorf("la respuesta no tiene por que llevar el contenido: %s", rec.Body)
	}

	if rec := get(t, h, "/api/jobs/no-existe", "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("un id desconocido deberia dar 404, dio %d", rec.Code)
	}
}

func mustWd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
