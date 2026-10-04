// Command collatech-agent es el agente local de impresion ESC/POS.
//
// Sin argumentos arranca el servidor usando configs/config.json junto al
// ejecutable. Con --install se registra como servicio del sistema (servicio
// de Windows, unidad systemd en Linux, demonio launchd en macOS).
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"collatech-agent/internal/api"
	"collatech-agent/internal/config"
	"collatech-agent/internal/logs"
	"collatech-agent/internal/printers"
	"collatech-agent/internal/queue"
)

const serviceName = "CollaTechAgent"

// version la fija el build con -ldflags "-X main.version=...".
var version = "dev"

type options struct {
	config    string
	dataDir   string
	host      string
	port      int
	token     string
	install   bool
	uninstall bool
	version   bool
}

func parseFlags() options {
	var o options
	flag.StringVar(&o.config, "config", "", "ruta del archivo de configuracion (por defecto configs/config.json junto al ejecutable)")
	flag.StringVar(&o.dataDir, "data-dir", "", "carpeta para logs/ y storage/ (por defecto junto al ejecutable)")
	flag.StringVar(&o.host, "host", "", "direccion de escucha; sobreescribe la del archivo de configuracion")
	flag.IntVar(&o.port, "port", 0, "puerto de escucha; sobreescribe el del archivo de configuracion")
	flag.StringVar(&o.token, "token", "", "token de acceso desde la red; sobreescribe el del archivo de configuracion")
	flag.BoolVar(&o.install, "install", false, "instalar como servicio del sistema y arrancarlo")
	flag.BoolVar(&o.uninstall, "uninstall", false, "detener y quitar el servicio del sistema")
	flag.BoolVar(&o.version, "version", false, "mostrar la version y salir")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "CollaTech Agent %s - agente de impresion ESC/POS\n\nUso:\n  %s [opciones]\n\nOpciones:\n", version, os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nEjemplos:\n"+
			"  %s                      arrancar con la configuracion de al lado\n"+
			"  %s --port 8080          arrancar en otro puerto\n"+
			"  %s --install            instalar como servicio del sistema\n"+
			"  %s --uninstall          quitar el servicio\n", os.Args[0], os.Args[0], os.Args[0], os.Args[0])
	}
	flag.Parse()
	return o
}

func main() {
	opts := parseFlags()

	if opts.version {
		fmt.Printf("CollaTech Agent %s\n", version)
		return
	}

	// Lo que venga en --config y --data-dir se resuelve contra el directorio
	// desde el que se llamo, y hay que hacerlo ANTES del Chdir de abajo.
	// Si no, "--data-dir ." apuntaba a la carpeta del ejecutable y el agente
	// leia y escribia en otro sitio del que el usuario creia, sin decir nada.
	if wd, err := os.Getwd(); err == nil {
		opts.config = desde(wd, opts.config)
		opts.dataDir = desde(wd, opts.dataDir)
	}

	// Las rutas relativas por defecto (configs/, logs/, storage/) si se
	// resuelven junto al ejecutable. Importa sobre todo para los gestores de
	// servicios, que arrancan con otro directorio de trabajo.
	if dir := config.ExecutableDir(); dir != "" {
		_ = os.Chdir(dir)
	}
	paths := config.DefaultPaths("").WithConfig(opts.config).WithDataDir(opts.dataDir)

	switch {
	case opts.install:
		// Al instalar se usan las rutas del sistema (ProgramFiles, /etc +
		// /var/lib, /usr/local), salvo que se indiquen a mano.
		sys := config.SystemPaths().WithConfig(opts.config).WithDataDir(opts.dataDir)
		if err := installService(sys); err != nil {
			log.Fatalf("no se pudo instalar el servicio: %v", err)
		}
		fmt.Println("Servicio instalado y en marcha.")
		return
	case opts.uninstall:
		if err := uninstallService(); err != nil {
			log.Fatalf("no se pudo quitar el servicio: %v", err)
		}
		fmt.Println("Servicio detenido y eliminado.")
		return
	}

	// En Windows, runService detecta si nos arranco el Service Control
	// Manager y en ese caso bloquea hasta que nos paren. En otros sistemas
	// devuelve false y seguimos en modo interactivo (systemd y launchd
	// supervisan el proceso directamente).
	if handled, err := runService(paths, opts); err != nil {
		log.Fatalf("error del servicio: %v", err)
	} else if handled {
		return
	}
	runInteractive(paths, opts)
}

func runInteractive(paths config.Paths, opts options) {
	agent, err := startServer(paths, opts)
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

func startServer(paths config.Paths, opts options) (*agent, error) {
	cfg, err := config.Load(paths.Config)
	if err != nil {
		return nil, fmt.Errorf("no se pudo leer %s: %w", paths.Config, err)
	}
	// Los flags mandan sobre el archivo.
	if opts.host != "" {
		cfg.Host = opts.host
	}
	if opts.port > 0 {
		cfg.Port = opts.port
	}
	if opts.token != "" {
		cfg.AuthToken = opts.token
	}

	logger, err := logs.NewJSONLoggerLevel(paths.Logs, cfg.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir el log en %s: %w", paths.Logs, err)
	}

	// En modo red hace falta un token: sin el, cualquiera en la red puede
	// imprimir, abrir el cajon de dinero y reescribir la configuracion. Si no
	// hay ninguno se genera y se guarda, para no dejar la instalacion abierta
	// ni obligar a editar el JSON a mano.
	if cfg.AllowRemote && cfg.AuthToken == "" {
		if token, tokenErr := config.NewToken(); tokenErr != nil {
			logger.Error("auth_token_failed", map[string]any{"error": tokenErr.Error()})
		} else {
			cfg.AuthToken = token
			if saveErr := config.Save(paths.Config, cfg); saveErr != nil {
				logger.Error("auth_token_save_failed", map[string]any{"error": saveErr.Error()})
			}
			logger.Info("auth_token_generated", nil)
			log.Printf("Se genero un token de acceso para la red. Lo tienes en http://localhost:%d/panel", cfg.Port)
		}
	}

	if cfg.AllowRemote && len(cfg.TrustedIPs) > 0 {
		log.Printf("AVISO: estas maquinas usan la API SIN token: %s. "+
			"Es para clientes antiguos mientras se migran; quitalas de trusted_ips en cuanto puedas.",
			strings.Join(cfg.TrustedIPs, ", "))
		logger.Info("trusted_ips_activas", map[string]any{"redes": cfg.TrustedIPs})
	}

	manager := printers.NewManager(logger)
	// Los destinos con ruta relativa cuentan desde la carpeta de datos, no
	// desde la del ejecutable, que es donde deja el Chdir de mas arriba.
	manager.SetBaseDir(paths.DataDir)
	queueManager := queue.New(queue.Options{
		Printers:   manager,
		Logger:     logger,
		Workers:    cfg.Queue.Workers,
		MaxRetries: cfg.Queue.MaxRetries,
		StorageDir: paths.Storage,
	})
	queueManager.Start()

	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	server := &http.Server{
		Addr:              addr,
		Handler:           apiServer(cfg, paths, manager, queueManager, logger).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// El socket se abre de forma sincrona. Antes el Listen ocurria dentro de
	// la goroutine, asi que un puerto ocupado solo dejaba una linea en el log
	// mientras el servicio le informaba "Running" al gestor.
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
	}

	scheme := "http"
	if cfg.TLS.Enabled {
		scheme = "https"
	}
	log.Printf("CollaTech Agent %s escuchando en %s://%s", version, scheme, addr)
	logger.Info("server_started", map[string]any{"addr": addr, "tls": cfg.TLS.Enabled, "version": version})

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

// desde convierte una ruta relativa en absoluta contra base. Una ruta vacia
// se deja igual: significa "usa el valor por defecto".
func desde(base, p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

// apiServer arma el servidor y le pasa la version, que el panel y la pagina
// del SDK muestran.
func apiServer(cfg config.Config, paths config.Paths, manager *printers.Manager, q *queue.Manager, logger *logs.Logger) *api.Server {
	srv := api.NewServer(cfg, paths, manager, q, logger)
	srv.Version = version
	return srv
}
