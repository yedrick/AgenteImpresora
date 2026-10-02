package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths agrupa donde vive cada cosa. Antes estaban escritas a mano y
// repetidas por todo el codigo ("configs/config.json", "storage/settings.json"
// siete veces, "logs", "LOGO.png"), lo que ademas impedia empaquetar el
// agente al estilo de Linux, con la configuracion en /etc y los datos en
// /var/lib.
type Paths struct {
	Config   string // archivo de configuracion
	DataDir  string // raiz de los datos mutables
	Logs     string // carpeta de logs
	Storage  string // carpeta de estado (cola y ajustes)
	Settings string // archivo de ajustes
	Logo     string // logo para /api/print/logo
}

// DefaultPaths resuelve las rutas a partir de un directorio base. En Windows
// y en una ejecucion portatil el base es la carpeta del ejecutable; en una
// instalacion de Linux se le pasa /var/lib/collatech-agent.
func DefaultPaths(base string) Paths {
	if base == "" {
		base = ExecutableDir()
	}
	return Paths{
		Config:   filepath.Join(base, "configs", "config.json"),
		DataDir:  base,
		Logs:     filepath.Join(base, "logs"),
		Storage:  filepath.Join(base, "storage"),
		Settings: filepath.Join(base, "storage", "settings.json"),
		Logo:     filepath.Join(base, "LOGO.png"),
	}
}

// WithDataDir mueve los datos mutables a otra carpeta, dejando el archivo de
// configuracion donde este. Es lo que necesita una instalacion de sistema:
// configuracion en /etc, datos en /var/lib.
func (p Paths) WithDataDir(dir string) Paths {
	if dir == "" {
		return p
	}
	p.DataDir = dir
	p.Logs = filepath.Join(dir, "logs")
	p.Storage = filepath.Join(dir, "storage")
	p.Settings = filepath.Join(dir, "storage", "settings.json")
	return p
}

// WithConfig cambia solo el archivo de configuracion.
func (p Paths) WithConfig(file string) Paths {
	if file != "" {
		p.Config = file
	}
	return p
}

// ExecutableDir es la carpeta donde esta el binario. Importa sobre todo para
// el Service Control Manager de Windows, que arranca los servicios con
// C:\Windows\System32 como directorio de trabajo.
func ExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		if wd, wdErr := os.Getwd(); wdErr == nil {
			return wd
		}
		return "."
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// SystemPaths son las rutas de una instalacion de sistema, por plataforma.
func SystemPaths() Paths {
	switch runtime.GOOS {
	case "windows":
		base := filepath.Join(os.Getenv("ProgramFiles"), "CollaTech Agent")
		return DefaultPaths(base)
	case "darwin":
		p := DefaultPaths("/usr/local/share/collatech-agent")
		p.Config = "/usr/local/etc/collatech-agent/config.json"
		return p.WithDataDir("/usr/local/var/collatech-agent")
	default:
		p := DefaultPaths("/usr/share/collatech-agent")
		p.Config = "/etc/collatech-agent/config.json"
		return p.WithDataDir("/var/lib/collatech-agent")
	}
}
