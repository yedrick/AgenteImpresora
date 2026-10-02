package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"

	"collatech-agent/internal/api"
	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/profiles"
	"collatech-agent/internal/queue"
	"collatech-agent/internal/render"
)

const serviceName = "CollaTechAgent"

func main() {
	runtime.GOMAXPROCS(1) // minimo uso de CPU
	chdirToExecutableDir()

	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("no se pudo determinar el modo de ejecucion: %v", err)
	}
	if isService {
		// Started by the Windows Service Control Manager (auto-start at
		// boot, before any user logs in). Blocks until the SCM stops us.
		if err := svc.Run(serviceName, &agentService{}); err != nil {
			log.Fatalf("service run: %v", err)
		}
		return
	}
	runInteractive()
}

// chdirToExecutableDir makes relative paths (configs/, logs/, storage/,
// templates/, certs/) resolve next to the executable regardless of how it
// was launched. This matters most for the Windows Service Control Manager,
// which by default starts services with C:\Windows\System32 as the working
// directory.
func chdirToExecutableDir() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if dir := filepath.Dir(exe); dir != "" {
		_ = os.Chdir(dir)
	}
}

func runInteractive() {
	server, logger, queueManager, err := startServer()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer queueManager.Stop()
	defer logger.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownServer(server, logger)
}

func startServer() (*http.Server, *logs.Logger, *queue.Manager, error) {
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load config: %w", err)
	}

	logger, err := logs.NewJSONLogger("logs")
	if err != nil {
		return nil, nil, nil, fmt.Errorf("create logger: %w", err)
	}

	profileStore, err := profiles.LoadDir("profiles")
	if err != nil {
		logger.Error("profiles_load_failed", map[string]any{"error": err.Error()})
	}

	manager := printers.NewManager(logger, profileStore)
	renderer := render.NewRenderer()
	queueManager := queue.NewManager(manager, logger, cfg.Queue.Workers, cfg.Queue.MaxRetries)
	queueManager.Start()

	server := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           api.NewServer(cfg, manager, queueManager, renderer, logger).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("server_started", map[string]any{"addr": server.Addr, "tls": cfg.TLS.Enabled})
		var serveErr error
		if cfg.TLS.Enabled && cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
			log.Printf("Iniciando con HTTPS en %s", server.Addr)
			serveErr = server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		} else {
			log.Printf("Iniciando con HTTP en %s (sin TLS)", server.Addr)
			serveErr = server.ListenAndServe()
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("server_failed", map[string]any{"error": serveErr.Error()})
		}
	}()

	return server, logger, queueManager, nil
}

func shutdownServer(server *http.Server, logger *logs.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	logger.Info("server_stopped", nil)
}

// agentService implements svc.Handler so the agent registers properly with
// the Windows Service Control Manager: reports Running once the HTTP server
// is up, and shuts down cleanly on Stop/Shutdown requests.
type agentService struct{}

func (a *agentService) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown

	s <- svc.Status{State: svc.StartPending}

	server, logger, queueManager, err := startServer()
	if err != nil {
		s <- svc.Status{State: svc.Stopped}
		return true, 1
	}
	defer queueManager.Stop()
	defer logger.Close()

	s <- svc.Status{State: svc.Running, Accepts: accepted}

	for req := range r {
		switch req.Cmd {
		case svc.Interrogate:
			s <- req.CurrentStatus
		case svc.Stop, svc.Shutdown:
			s <- svc.Status{State: svc.StopPending}
			shutdownServer(server, logger)
			s <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
	return false, 0
}
