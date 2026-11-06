package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"collatech-agent/internal/config"
)

// El interruptor del panel, con el agente arrancado de verdad: la peticion
// recibe su respuesta, el agente sigue respondiendo y config.json cambia.
func TestArrancarYActivarRed(t *testing.T) {
	dir := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	puerto := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	paths := config.DefaultPaths(dir)
	cfg := config.Default()
	cfg.Port = puerto
	if err := config.Save(paths.Config, cfg); err != nil {
		t.Fatal(err)
	}
	paths = paths.WithDataDir(filepath.Join(dir, "datos"))

	ag, err := startServer(paths, options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ag.shutdown()
		ag.queue.Stop()
		ag.logger.Close()
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", puerto)
	c := http.Client{Timeout: 5 * time.Second}
	r, err := c.Post(base+"/api/red", "application/json", strings.NewReader(`{"activo":true}`))
	if err != nil {
		t.Fatal(err)
	}
	cuerpo, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("activar la red: %d %s", r.StatusCode, cuerpo)
	}
	r, err = c.Get(base + "/health")
	if err != nil {
		t.Fatalf("tras activar la red el agente tiene que seguir respondiendo: %v", err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("/health tras activar: %d", r.StatusCode)
	}
	guardada, err := config.Load(paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if guardada.Host != "0.0.0.0" || !guardada.AllowRemote {
		t.Fatalf("config.json no quedo abierta a la red: host=%q remote=%v", guardada.Host, guardada.AllowRemote)
	}
}
