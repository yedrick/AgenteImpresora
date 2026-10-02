//go:build windows

package main

import (
	"golang.org/x/sys/windows/svc"
)

// runService registra el agente con el Service Control Manager cuando es el
// SCM quien nos arranca (auto-arranque al encender, antes de que nadie inicie
// sesion). Devuelve handled=true si se ejecuto como servicio.
func runService() (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false, err
	}
	if !isService {
		return false, nil
	}
	if err := svc.Run(serviceName, &agentService{}); err != nil {
		return true, err
	}
	return true, nil
}

// agentService implementa svc.Handler: informa Running cuando el servidor HTTP
// esta realmente escuchando, y se apaga limpiamente ante Stop/Shutdown.
type agentService struct{}

func (a *agentService) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}

	ag, err := startServer()
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
