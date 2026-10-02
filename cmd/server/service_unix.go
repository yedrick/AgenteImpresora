//go:build !windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"collatech-agent/internal/config"
)

// runService no hace nada fuera de Windows: systemd y launchd supervisan el
// proceso directamente, sin protocolo de control que implementar.
func runService(config.Paths, options) (bool, error) { return false, nil }

const (
	systemdUnit  = "/etc/systemd/system/collatech-agent.service"
	launchdPlist = "/Library/LaunchDaemons/com.collatech.agent.plist"
	installedBin = "collatech-agent"
)

func installService(paths config.Paths) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("hay que ejecutarlo como root: prueba con 'sudo %s --install'", os.Args[0])
	}

	binDir := "/usr/local/bin"
	if runtime.GOOS == "linux" {
		binDir = "/usr/local/bin"
	}
	binPath := filepath.Join(binDir, installedBin)

	for _, dir := range []string{binDir, filepath.Dir(paths.Config), paths.DataDir, paths.Logs, paths.Storage} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("no se pudo crear %s: %w", dir, err)
		}
	}
	if err := copyExecutable(binPath); err != nil {
		return err
	}
	if err := writeDefaultConfig(paths.Config); err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(binPath, paths)
	default:
		return installSystemd(binPath, paths)
	}
}

func uninstallService() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("hay que ejecutarlo como root: prueba con 'sudo %s --uninstall'", os.Args[0])
	}
	switch runtime.GOOS {
	case "darwin":
		_ = run("launchctl", "unload", "-w", launchdPlist)
		return os.Remove(launchdPlist)
	default:
		_ = run("systemctl", "stop", "collatech-agent")
		_ = run("systemctl", "disable", "collatech-agent")
		if err := os.Remove(systemdUnit); err != nil && !os.IsNotExist(err) {
			return err
		}
		return run("systemctl", "daemon-reload")
	}
}

func installSystemd(binPath string, paths config.Paths) error {
	// ProtectSystem=strict deja todo el sistema de archivos en solo lectura
	// salvo lo que se declare: el agente solo necesita escribir sus datos y,
	// la primera vez, el token en su configuracion.
	unit := fmt.Sprintf(`[Unit]
Description=CollaTech Agent - impresion ESC/POS
Documentation=https://github.com/collatech/agent
After=network-online.target cups.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s --config %s --data-dir %s
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=%s %s
# Acceso a impresoras USB y puertos serie
SupplementaryGroups=lp dialout
DeviceAllow=char-usb/lp rw
DeviceAllow=char-ttyUSB rw

[Install]
WantedBy=multi-user.target
`, binPath, paths.Config, paths.DataDir, paths.DataDir, filepath.Dir(paths.Config))

	if err := os.WriteFile(systemdUnit, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", systemdUnit, err)
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "enable", "collatech-agent"); err != nil {
		return err
	}
	return run("systemctl", "restart", "collatech-agent")
}

func installLaunchd(binPath string, paths config.Paths) error {
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.collatech.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>--config</string><string>%s</string>
    <string>--data-dir</string><string>%s</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardErrorPath</key><string>%s/launchd.log</string>
</dict>
</plist>
`, binPath, paths.Config, paths.DataDir, paths.Logs)

	if err := os.WriteFile(launchdPlist, []byte(plist), 0o644); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", launchdPlist, err)
	}
	_ = run("launchctl", "unload", launchdPlist)
	return run("launchctl", "load", "-w", launchdPlist)
}

// copyExecutable copia el binario en marcha a su ubicacion definitiva.
func copyExecutable(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(src); err == nil {
		src = resolved
	}
	if sameFile(src, dst) {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// Se escribe a un temporal y se renombra: sustituir un binario en uso
	// falla, y asi la instalacion es atomica.
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", tmp, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// writeDefaultConfig crea la configuracion inicial si no existe, ya con un
// token generado: asi la instalacion nunca queda abierta a la red.
func writeDefaultConfig(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	cfg := config.Default()
	cfg.Host = "0.0.0.0"
	cfg.AllowRemote = true
	cfg.AllowedCORS = []string{"http://localhost:18743", "http://127.0.0.1:18743"}
	cfg.Queue.Workers = 4
	token, err := config.NewToken()
	if err != nil {
		return err
	}
	cfg.AuthToken = token
	if err := config.Save(path, cfg); err != nil {
		return err
	}
	fmt.Printf("\nTOKEN DE ACCESO PARA LA RED:\n  %s\n\n"+
		"Los sistemas que impriman desde OTRA maquina deben enviarlo en la\n"+
		"cabecera 'Authorization: Bearer <token>'. Desde esta no hace falta.\n\n", token)
	return nil
}

func run(name string, args ...string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("no se encontro %q: instala el servicio a mano", name)
	}
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}
