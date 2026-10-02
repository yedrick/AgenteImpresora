package printers

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"collatech-agent/internal/logs"
	"collatech-agent/internal/profiles"
)

type Printer interface {
	Connect() error
	Disconnect() error
	Print([]byte) error
	Status() error
}

type Info struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Address string `json:"address,omitempty"`
	Online  bool   `json:"online"`
}

type Manager struct {
	logger   *logs.Logger
	profiles *profiles.Store
}

func NewManager(logger *logs.Logger, profileStore *profiles.Store) *Manager {
	return &Manager{logger: logger, profiles: profileStore}
}

func (m *Manager) List() []Info {
	var out []Info
	if runtime.GOOS == "windows" {
		out = append(out, listWindowsPrinters()...)
	}
	out = append(out, detectSerialPorts()...)
	return out
}

func (m *Manager) Get(name string) (Printer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("printer is required")
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "tcp://"):
		return NewTCPPrinter(strings.TrimPrefix(name, "tcp://"), 5*time.Second), nil
	case strings.Contains(name, ":") && !strings.Contains(name, `\`):
		return NewTCPPrinter(name, 5*time.Second), nil
	case strings.HasPrefix(lower, "com"):
		return NewFilePrinter(`\\.\`+strings.ToUpper(name), "COM"), nil
	case strings.HasPrefix(lower, "usb"):
		return NewFilePrinter(name, "USB"), nil
	default:
		return NewWindowsPrinter(name), nil
	}
}

func (m *Manager) Print(name string, payload []byte) error {
	p, err := m.Get(name)
	if err != nil {
		return err
	}
	if err := p.Connect(); err != nil {
		return err
	}
	defer p.Disconnect()
	if err := p.Print(payload); err != nil {
		return err
	}
	if m.logger != nil {
		m.logger.Info("print_sent", map[string]any{"printer": name, "bytes": len(payload)})
	}
	return nil
}

func listWindowsPrinters() []Info {
	return enumWindowsPrinters()
}

func detectSerialPorts() []Info {
	var out []Info
	if runtime.GOOS != "windows" {
		return out
	}
	for i := 1; i <= 32; i++ {
		name := fmt.Sprintf("COM%d", i)
		path := `\\.\` + name
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err == nil {
			_ = f.Close()
			out = append(out, Info{Name: name, Type: "com", Address: path, Online: true})
		}
	}
	return out
}

type TCPPrinter struct {
	addr    string
	timeout time.Duration
	conn    net.Conn
}

func NewTCPPrinter(addr string, timeout time.Duration) *TCPPrinter {
	return &TCPPrinter{addr: addr, timeout: timeout}
}

func (p *TCPPrinter) Connect() error {
	conn, err := net.DialTimeout("tcp", p.addr, p.timeout)
	if err != nil {
		return err
	}
	p.conn = conn
	return nil
}

func (p *TCPPrinter) Disconnect() error {
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}

func (p *TCPPrinter) Print(data []byte) error {
	if p.conn == nil {
		return fmt.Errorf("tcp printer is not connected")
	}
	_, err := p.conn.Write(data)
	return err
}

func (p *TCPPrinter) Status() error { return nil }

type FilePrinter struct {
	path string
	typ  string
	file *os.File
}

func NewFilePrinter(path, typ string) *FilePrinter {
	return &FilePrinter{path: path, typ: typ}
}

func (p *FilePrinter) Connect() error {
	f, err := os.OpenFile(p.path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	p.file = f
	return nil
}

func (p *FilePrinter) Disconnect() error {
	if p.file == nil {
		return nil
	}
	return p.file.Close()
}

func (p *FilePrinter) Print(data []byte) error {
	if p.file == nil {
		return fmt.Errorf("%s printer is not connected", p.typ)
	}
	_, err := p.file.Write(data)
	return err
}

func (p *FilePrinter) Status() error { return nil }
