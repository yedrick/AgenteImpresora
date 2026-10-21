package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// DefaultPort es el puerto de escucha del agente. Estaba escrito a mano en
// veintiseis sitios entre el servidor y el instalador.
const DefaultPort = 18743

type Config struct {
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	LogLevel    string   `json:"log_level"`
	AllowedCORS []string `json:"allowed_cors"`
	AllowRemote bool     `json:"allow_remote"`
	// AuthToken se exige en /api/* a las peticiones que no vienen de la propia
	// PC. Si allow_remote esta activo y esta vacio, el agente genera uno al
	// arrancar: hasta ahora cualquiera en la LAN podia imprimir sin mas.
	AuthToken string `json:"auth_token"`

	// TrustedIPs son maquinas concretas que pueden usar la API sin token.
	//
	// Existe para convivir con clientes ya desplegados que no saben
	// mandarlo, mientras se migran. Admite una IP ("192.168.1.20") o un
	// rango ("192.168.1.0/24").
	//
	// Es una lista de maquinas nombradas a proposito, no un interruptor
	// para apagar la autenticacion: un rango que lo abarque todo (/0) se
	// rechaza al arrancar. Las rutas de administracion siguen siendo solo
	// para la propia PC del agente, aunque la IP este en esta lista.
	TrustedIPs   []string `json:"trusted_ips,omitempty"`
	MaxPrintSize int64    `json:"max_print_size"`
	Queue        Queue    `json:"queue"`
	TLS          TLS      `json:"tls"`

	// Cloud conecta el agente con un backend en internet (por ejemplo
	// Laravel) que no puede llegar a la red del local.
	Cloud Cloud `json:"cloud"`
}

// Cloud es la conexion con el backend. Es el agente quien llama: pregunta
// cada pocos segundos si hay trabajos y avisa de como estan las impresoras,
// asi que no hace falta abrir puertos en el router del local.
type Cloud struct {
	// URL base de las rutas del backend, por ejemplo
	// https://mi-app.com/api/collatech. Vacia, el conector esta apagado.
	URL string `json:"url"`
	// Token identifica a este local ante el backend.
	Token string `json:"token"`
	// PollSeconds es cada cuanto pregunta si hay trabajos. 3 por defecto.
	PollSeconds int `json:"poll_seconds,omitempty"`
	// HeartbeatSeconds es cada cuanto informa de que sigue encendido y de
	// como estan las impresoras. 30 por defecto.
	HeartbeatSeconds int `json:"heartbeat_seconds,omitempty"`
}

// Enabled dice si hay datos para conectarse.
func (c Cloud) Enabled() bool { return c.URL != "" && c.Token != "" }

type Queue struct {
	Workers    int `json:"workers"`
	MaxRetries int `json:"max_retries"`
}

type TLS struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file"`
	KeyFile  string `json:"key_file"`
}

func Default() Config {
	return Config{
		Host:     "127.0.0.1",
		Port:     DefaultPort,
		LogLevel: "info",
		// Sin "null": ese origen lo manda cualquier iframe con sandbox, asi
		// que admitirlo dejaba que una web cualquiera leyera /api/token y se
		// llevara el token de la red.
		AllowedCORS:  []string{"http://localhost", "http://127.0.0.1"},
		AllowRemote:  false,
		MaxPrintSize: 2 * 1024 * 1024,
		Queue:        Queue{Workers: 1, MaxRetries: 2},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // strip BOM
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	if cfg.Host == "" {
		cfg.Host = Default().Host
	}
	if cfg.MaxPrintSize <= 0 {
		cfg.MaxPrintSize = Default().MaxPrintSize
	}
	if cfg.Queue.Workers <= 0 {
		cfg.Queue.Workers = 1
	}
	if cfg.Queue.MaxRetries <= 0 {
		cfg.Queue.MaxRetries = 2
	}
	return cfg, nil
}

// Save reescribe el archivo de configuracion. Se usa para persistir el token
// generado automaticamente la primera vez que se arranca en modo LAN.
func Save(path string, cfg Config) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// NewToken genera un token compartido de 32 caracteres hexadecimales.
func NewToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
