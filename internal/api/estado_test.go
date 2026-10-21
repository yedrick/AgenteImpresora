package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"collatech-agent/internal/config"
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

// La conexion con la nube se configura solo desde la propia PC, se guarda en
// el archivo de configuracion y nunca devuelve el token entero.
func TestConfigurarNube(t *testing.T) {
	ajustar := func(c *config.Config) { c.AllowRemote = true; c.AuthToken = "lan" }
	h := newTestServer(t, ajustar)
	// Como en una instalacion de verdad, la configuracion esta en disco: lo
	// que no sea de la nube tiene que seguir igual despues de guardar.
	enDisco := config.Default()
	ajustar(&enDisco)
	if err := config.Save(config.DefaultPaths(mustWd(t)).Config, enDisco); err != nil {
		t.Fatal(err)
	}

	cuerpo := `{"url":"https://mi-app.com/api/collatech/","token":"abcdef123456"}`
	if rec := postConToken(t, h, "/api/cloud", cuerpo, "192.168.1.50:4000", "lan"); rec.Code != http.StatusForbidden {
		t.Fatalf("desde la red tenia que estar prohibido, dio %d", rec.Code)
	}
	if rec := get(t, h, "/api/cloud", "192.168.1.50:4000", "lan"); rec.Code != http.StatusForbidden {
		t.Fatalf("leerla desde la red tenia que estar prohibido, dio %d", rec.Code)
	}

	rec := post(t, h, "/api/cloud", cuerpo, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("guardar: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "abcdef123456") {
		t.Fatalf("la respuesta devuelve el token entero: %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"token_hint":"...3456"`) || !strings.Contains(rec.Body.String(), `"url":"https://mi-app.com/api/collatech"`) {
		t.Fatalf("respuesta: %s", rec.Body)
	}

	disco, err := config.Load(config.DefaultPaths(mustWd(t)).Config)
	if err != nil {
		t.Fatal(err)
	}
	if disco.Cloud.Token != "abcdef123456" || disco.Cloud.URL != "https://mi-app.com/api/collatech" {
		t.Fatalf("no se guardo en el archivo: %+v", disco.Cloud)
	}
	if disco.AuthToken != "lan" {
		t.Fatalf("guardar la nube piso el resto de la configuracion: %+v", disco)
	}

	// Cambiar la URL sin mandar el token conserva el que habia.
	if rec := post(t, h, "/api/cloud", `{"url":"https://otra.com/x"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("cambiar URL: %d %s", rec.Code, rec.Body)
	}
	disco, _ = config.Load(config.DefaultPaths(mustWd(t)).Config)
	if disco.Cloud.Token != "abcdef123456" {
		t.Fatal("cambiar la URL borro el token")
	}

	for _, malo := range []string{`{"url":"mi-app.com"}`, `{"url":"ftp://x.com","token":"t"}`, `{"url":"https://x.com","token":""}`} {
		if rec := post(t, h, "/api/cloud", malo, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s tenia que rechazarse, dio %d", malo, rec.Code)
		}
	}
}

func postConToken(t *testing.T, h http.Handler, path, body, remote, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Autorizar una web despues de conectar la nube no puede borrar la conexion:
// /api/cors guardaba la configuracion de memoria, que no la tenia.
func TestAutorizarWebNoBorraLaNube(t *testing.T) {
	h := newTestServer(t, nil)
	if err := config.Save(config.DefaultPaths(mustWd(t)).Config, config.Default()); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "/api/cloud", `{"url":"https://mi-app.com/api/collatech","token":"abcdef123456"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("nube: %d %s", rec.Code, rec.Body)
	}
	if rec := post(t, h, "/api/cors", `["http://localhost:5173"]`, ""); rec.Code != http.StatusOK {
		t.Fatalf("cors: %d %s", rec.Code, rec.Body)
	}
	disco, err := config.Load(config.DefaultPaths(mustWd(t)).Config)
	if err != nil {
		t.Fatal(err)
	}
	if disco.Cloud.Token != "abcdef123456" {
		t.Fatalf("autorizar una web borro la conexion con la nube: %+v", disco.Cloud)
	}
	if len(disco.AllowedCORS) != 1 || disco.AllowedCORS[0] != "http://localhost:5173" {
		t.Fatalf("no se guardo la web: %v", disco.AllowedCORS)
	}
}
