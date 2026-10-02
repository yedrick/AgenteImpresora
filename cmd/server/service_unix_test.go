//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"collatech-agent/internal/config"
)

func TestUnidadSystemd(t *testing.T) {
	paths := config.SystemPaths()
	unit := systemdUnitText("/usr/local/bin/collatech-agent", paths)

	debeContener := []string{
		"ExecStart=/usr/local/bin/collatech-agent --config " + paths.Config + " --data-dir " + paths.DataDir,
		"Restart=on-failure",
		"WantedBy=multi-user.target",
		"After=network-online.target cups.service",
		// Endurecido, pero con permiso de escritura donde hace falta.
		"ProtectSystem=strict",
		"NoNewPrivileges=true",
		"ReadWritePaths=" + paths.DataDir + " " + filepath.Dir(paths.Config),
		// Acceso a impresoras USB y puertos serie.
		"SupplementaryGroups=lp dialout",
	}
	for _, s := range debeContener {
		if !strings.Contains(unit, s) {
			t.Fatalf("falta %q en la unidad:\n%s", s, unit)
		}
	}
	// Si ProtectSystem deja /etc en solo lectura, el agente no podria guardar
	// el token que genera al arrancar.
	if !strings.Contains(unit, "ReadWritePaths") {
		t.Fatal("sin ReadWritePaths el agente no podria guardar su token")
	}
}

func TestPlistLaunchd(t *testing.T) {
	paths := config.SystemPaths()
	plist := launchdPlistText("/usr/local/bin/collatech-agent", paths)
	for _, s := range []string{
		"<key>Label</key><string>com.collatech.agent</string>",
		"<string>/usr/local/bin/collatech-agent</string>",
		"<key>RunAtLoad</key><true/>",
		"<key>KeepAlive</key><true/>",
	} {
		if !strings.Contains(plist, s) {
			t.Fatalf("falta %q en el plist:\n%s", s, plist)
		}
	}
	if !strings.HasPrefix(plist, "<?xml") {
		t.Fatal("el plist deberia empezar por la declaracion XML")
	}
}

// La configuracion inicial nunca puede quedar abierta a la red sin token.
func TestConfiguracionInicialLlevaToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "etc", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeDefaultConfig(path); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg config.Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.AllowRemote {
		t.Fatal("una instalacion de sistema deberia permitir la red")
	}
	if len(cfg.AuthToken) != 32 {
		t.Fatalf("token de %d caracteres, esperaba 32", len(cfg.AuthToken))
	}
	for _, o := range cfg.AllowedCORS {
		if o == "*" {
			t.Fatal("la configuracion inicial no debe llevar CORS comodin")
		}
	}

	// Si ya existe, no se pisa: reinstalar no debe cambiar el token.
	antes := cfg.AuthToken
	if err := writeDefaultConfig(path); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	_ = json.Unmarshal(raw, &cfg)
	if cfg.AuthToken != antes {
		t.Fatal("reinstalar no deberia regenerar el token")
	}
}

func TestCopiarEjecutable(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "collatech-agent")
	if err := copyExecutable(dst); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("el binario copiado deberia ser ejecutable, tiene %v", info.Mode().Perm())
	}
	if info.Size() == 0 {
		t.Fatal("el binario copiado esta vacio")
	}
}
