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
	AuthToken    string `json:"auth_token"`
	MaxPrintSize int64  `json:"max_print_size"`
	Queue        Queue  `json:"queue"`
	TLS          TLS    `json:"tls"`
}

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
