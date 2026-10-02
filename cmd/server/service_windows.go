//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/svc"

	"collatech-agent/internal/config"
)

// runService registra el agente con el Service Control Manager cuando es el
// SCM quien nos arranca (auto-arranque al encender, antes de que nadie inicie
// sesion). Devuelve handled=true si se ejecuto como servicio.
func runService(paths config.Paths, opts options) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, err
	}
	if !isService {
		return false, nil
	}
	if err := svc.Run(serviceName, &agentService{paths: paths, opts: opts}); err != nil {
		return true, err
	}
	return true, nil
}

// agentService implementa svc.Handler: informa Running cuando el servidor HTTP
// esta realmente escuchando, y se apaga limpiamente ante Stop/Shutdown.
type agentService struct {
	paths config.Paths
	opts  options
}

func (a *agentService) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}

	ag, err := startServer(a.paths, a.opts)
	if err != nil {
		s <- svc.Status{State: svc.Stopped}
		return true, 1
	}
	defer ag.queue.Stop()
	defer ag.logger.Close()

	s <- svc.Status{State: svc.Running, Accepts: accepted}

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			s <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			ag.shutdown()
			s <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
	return false, 0
}

// systemTool devuelve la ruta absoluta en System32. Resolver "sc" por el PATH
// permitiria que un ejecutable con ese nombre, colocado antes en el PATH, se
// ejecutara con los privilegios del instalador.
func systemTool(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", name)
}

func sc(args ...string) error {
	cmd := exec.Command(systemTool("sc.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc %s: %s", args[0], string(out))
	}
	return nil
}

func installService(paths config.Paths) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = sc("stop", serviceName)
	_ = sc("delete", serviceName)

	// binPath lleva la ruta entrecomillada por si tiene espacios: sin
	// comillas, Windows interpretaria "C:\Program" como el ejecutable.
	bin := fmt.Sprintf(`"%s" --config "%s" --data-dir "%s"`, exe, paths.Config, paths.DataDir)
	if err := sc("create", serviceName, "binPath=", bin, "start=", "auto", "DisplayName=", "CollaTech Agent"); err != nil {
		return err
	}
	_ = sc("description", serviceName, "Agente de impresion ESC/POS CollaTech")
	_ = sc("failure", serviceName, "reset=", "86400", "actions=", "restart/5000/restart/5000/restart/5000")
	return sc("start", serviceName)
}

func uninstallService() error {
	_ = sc("stop", serviceName)
	return sc("delete", serviceName)
}
