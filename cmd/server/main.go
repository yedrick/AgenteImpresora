package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"collatech-agent/internal/api"
	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
)

const serviceName = "CollaTechAgent"

func main() {
	runtime.GOMAXPROCS(1) // minimo uso de CPU
	chdirToExecutableDir()

	// En Windows, runService detecta si nos arranco el Service Control
	// Manager y en ese caso bloquea hasta que nos paren. En otros sistemas
	// devuelve false y seguimos en modo interactivo.
	if handled, err := runService(); err != nil {
		log.Fatalf("service run: %v", err)
	} else if handled {
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
	agent, err := startServer()
	if err != nil {
		log.Fatalf("%v", err)
	}
	defer agent.queue.Stop()
	defer agent.logger.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	agent.shutdown()
}

// agent agrupa lo que hay que parar al apagar.
type agent struct {
	server *http.Server
	logger *logs.Logger
	queue  *queue.Manager
}

func startServer() (*agent, error) {
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	logger, err := logs.NewJSONLoggerLevel("logs", cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("create logger: %w", err)
	}

	// En modo LAN hace falta un token: hasta ahora cualquiera en la red podia
	// imprimir, abrir el cajon de dinero y reescribir la configuracion. Si no
	// hay ninguno se genera y se guarda, para no dejar la instalacion abierta
	// ni obligar a editar el JSON a mano.
	if cfg.AllowRemote && cfg.AuthToken == "" {
		token, tokenErr := config.NewToken()
		if tokenErr != nil {
			logger.Error("auth_token_failed", map[string]any{"error": tokenErr.Error()})
		} else {
			cfg.AuthToken = token
			if saveErr := config.Save("configs/config.json", cfg); saveErr != nil {
				logger.Error("auth_token_save_failed", map[string]any{"error": saveErr.Error()})
			}
			logger.Info("auth_token_generated", nil)
			log.Printf("Se genero un token de acceso para la red. Mira http://localhost:%d/panel para copiarlo.", cfg.Port)
		}
	}

	manager := printers.NewManager(logger)
	queueManager := queue.NewManager(manager, logger, cfg.Queue.Workers, cfg.Queue.MaxRetries)
	queueManager.Start()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           api.NewServer(cfg, manager, queueManager, logger).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Se abre el socket aqui, de forma sincrona. Antes el Listen ocurria
	// dentro de la goroutine, asi que un puerto ocupado solo dejaba una linea
	// en el log mientras el servicio le informaba "Running" al SCM.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		queueManager.Stop()
		logger.Error("server_failed", map[string]any{"error": err.Error(), "addr": addr})
		logger.Close()
		return nil, fmt.Errorf("no se pudo escuchar en %s: %w", addr, err)
	}
	if cfg.TLS.Enabled {
		cert, certErr := tls.LoadX509KeyPair(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if certErr != nil {
			ln.Close()
			queueManager.Stop()
			logger.Error("server_failed", map[string]any{"error": certErr.Error(), "cert": cfg.TLS.CertFile})
			logger.Close()
			return nil, fmt.Errorf("no se pudo cargar el certificado TLS: %w", certErr)
		}
		server.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		ln = tls.NewListener(ln, server.TLSConfig)
		log.Printf("Iniciando con HTTPS en %s", addr)
	} else {
		log.Printf("Iniciando con HTTP en %s (sin TLS)", addr)
	}

	logger.Info("server_started", map[string]any{"addr": addr, "tls": cfg.TLS.Enabled})
	go func() {
		if serveErr := server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			logger.Error("server_failed", map[string]any{"error": serveErr.Error()})
		}
	}()

	return &agent{server: server, logger: logger, queue: queueManager}, nil
}

func (a *agent) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = a.server.Shutdown(ctx)
	a.logger.Info("server_stopped", nil)
}
