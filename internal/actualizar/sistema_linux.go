//go:build linux

package actualizar

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// UnidadVigia es la unidad systemd que recoge las ordenes de actualizacion.
// El servicio principal esta encerrado (ProtectSystem=strict) y no puede
// cambiar su propio ejecutable: solo deja la orden en su carpeta de datos, y
// esta unidad, que corre fuera del encierro, lanza el aplicador.
const UnidadVigia = "collatech-agent-actualizar.path"

type sistemaLinux struct {
	servicio string
}

// SistemaActual devuelve el sistema de esta maquina.
func SistemaActual(dataDir string) Sistema {
	return sistemaLinux{servicio: "collatech-agent"}
}

func (s sistemaLinux) Soportado() (bool, string) {
	dirs := []string{"/etc/systemd/system", "/lib/systemd/system", "/usr/lib/systemd/system"}
	if systemctlUsuario() {
		if home, err := os.UserHomeDir(); err == nil {
			dirs = append(dirs, filepath.Join(home, ".config", "systemd", "user"))
		}
	}
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, UnidadVigia)); err == nil {
			return true, ""
		}
	}
	return false, "Esta instalacion no tiene la pieza de systemd que hace el cambio (" + UnidadVigia +
		"). Instala una vez a mano el .deb o el paquete nuevo y despues ya vale el boton."
}

func (s sistemaLinux) RutaEjecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func (s sistemaLinux) Parar(ctx context.Context) error {
	return systemctl(ctx, "stop", s.servicio)
}

func (s sistemaLinux) Arrancar(ctx context.Context) error {
	return systemctl(ctx, "start", s.servicio)
}

// Lanzar no hace nada mas: la orden ya esta escrita y la unidad .path la
// recoge en cuanto aparece.
func (s sistemaLinux) Lanzar(string) error { return nil }

func systemctl(ctx context.Context, args ...string) error {
	if systemctlUsuario() {
		args = append([]string{"--user"}, args...)
	}
	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %v: %v: %s", args, err, out)
	}
	return nil
}
