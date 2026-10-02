package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
)

type Config struct {
	Host         string   `json:"host"`
	Port         int      `json:"port"`
	LogLevel     string   `json:"log_level"`
	AllowedCORS  []string `json:"allowed_cors"`
	AllowRemote  bool     `json:"allow_remote"`
	MaxPrintSize int64    `json:"max_print_size"`
	Queue        Queue    `json:"queue"`
	TLS          TLS      `json:"tls"`
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
		Host:         "127.0.0.1",
		Port:         18743,
		LogLevel:     "info",
		AllowedCORS:  []string{"http://localhost", "http://127.0.0.1", "null"},
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
		cfg.Port = 18743
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
