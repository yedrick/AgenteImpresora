//go:build windows

package actualizar

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const nombreServicio = "CollaTechAgent"

// Banderas de CreateProcess. El aplicador tiene que sobrevivir a que se pare
// el servicio que lo lanzo: sin consola, en su propio grupo y fuera del job
// del servicio, si Windows lo permite.
const (
	detachedProcess        = 0x00000008
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
)

type sistemaWindows struct {
	dataDir string
}

// SistemaActual devuelve el sistema de esta maquina.
func SistemaActual(dataDir string) Sistema {
	return sistemaWindows{dataDir: dataDir}
}

func abrirServicio() (*mgr.Mgr, *mgr.Service, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, nil, err
	}
	s, err := m.OpenService(nombreServicio)
	if err != nil {
		m.Disconnect()
		return nil, nil, err
	}
	return m, s, nil
}

func (w sistemaWindows) Soportado() (bool, string) {
	m, s, err := abrirServicio()
	if err != nil {
		return false, "El agente no esta instalado como servicio de Windows: se actualiza a mano con INSTALADOR.exe."
	}
	s.Close()
	m.Disconnect()
	return true, ""
}

// RutaEjecutable lee la ruta real del servicio: puede no estar en Program
// Files (los de 32 bits se instalaron con --install desde donde estuvieran).
func (w sistemaWindows) RutaEjecutable() (string, error) {
	m, s, err := abrirServicio()
	if err != nil {
		return "", err
	}
	defer m.Disconnect()
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return "", err
	}
	return rutaDeBinPath(cfg.BinaryPathName)
}

func (w sistemaWindows) Parar(ctx context.Context) error {
	return controlar(ctx, false)
}

func (w sistemaWindows) Arrancar(ctx context.Context) error {
	return controlar(ctx, true)
}

func controlar(ctx context.Context, arrancar bool) error {
	m, s, err := abrirServicio()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	defer s.Close()

	objetivo := svc.Stopped
	if arrancar {
		objetivo = svc.Running
		if err := s.Start(); err != nil {
			if st, qerr := s.Query(); qerr != nil || st.State != svc.Running {
				return fmt.Errorf("no se pudo arrancar el servicio: %w", err)
			}
		}
	} else if st, err := s.Query(); err == nil && st.State != svc.Stopped {
		if _, err := s.Control(svc.Stop); err != nil {
			return fmt.Errorf("no se pudo parar el servicio: %w", err)
		}
	}

	limite := time.Now().Add(30 * time.Second)
	for time.Now().Before(limite) {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == objetivo {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return fmt.Errorf("el servicio no llego a %v en 30 s", objetivo)
}

// Lanzar copia el ejecutable actual (el de confianza) y lo arranca aparte
// con la orden. No se usa el instalado directamente: en Windows no se puede
// reemplazar un .exe mientras se ejecuta.
func (w sistemaWindows) Lanzar(orden string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	aplicador := filepath.Join(filepath.Dir(orden), "aplicador.exe")
	if err := copiarArchivo(exe, aplicador); err != nil {
		return fmt.Errorf("no se pudo preparar el aplicador: %w", err)
	}
	arrancar := func(banderas uint32) error {
		cmd := exec.Command(aplicador, "--aplicar-actualizacion", orden)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: banderas}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	base := uint32(detachedProcess | createNewProcessGroup)
	if err := arrancar(base | createBreakawayFromJob); err != nil {
		// Si el job del servicio no deja salir, se lanza sin esa bandera:
		// el SCM no mata a los hijos al parar un servicio.
		return arrancar(base)
	}
	return nil
}

func copiarArchivo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
