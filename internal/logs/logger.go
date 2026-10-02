package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RetentionDays es cuantos dias de log se conservan. El agente corre como
// servicio durante semanas, asi que sin purga la carpeta crecia sin limite.
const RetentionDays = 30

type Logger struct {
	mu       sync.Mutex
	dir      string
	day      string
	file     *os.File
	onlyErrs bool
}

type Entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Event   string         `json:"event"`
	Details map[string]any `json:"details,omitempty"`
}

// NewJSONLogger abre el log del dia. level acepta "error" para registrar solo
// los fallos; cualquier otro valor registra todo. Antes el log_level del
// config se leia y no se usaba.
func NewJSONLogger(dir string) (*Logger, error) {
	return NewJSONLoggerLevel(dir, "info")
}

func NewJSONLoggerLevel(dir, level string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Logger{dir: dir, onlyErrs: strings.EqualFold(strings.TrimSpace(level), "error")}
	if err := l.rotate(time.Now()); err != nil {
		return nil, err
	}
	l.purge(time.Now())
	return l, nil
}

func (l *Logger) Info(event string, details map[string]any)  { l.write("info", event, details) }
func (l *Logger) Error(event string, details map[string]any) { l.write("error", event, details) }

func (l *Logger) write(level, event string, details map[string]any) {
	if l == nil || (l.onlyErrs && level != "error") {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	// El nombre del archivo se recalculaba solo al arrancar, asi que un
	// servicio encendido semanas lo escribia todo en el archivo del dia en
	// que se encendio.
	if err := l.rotate(now); err != nil || l.file == nil {
		return
	}
	_ = json.NewEncoder(l.file).Encode(Entry{
		Time:    now.Format(time.RFC3339Nano),
		Level:   level,
		Event:   event,
		Details: details,
	})
}

// rotate abre el archivo del dia si hace falta. El nombre y la marca de tiempo
// usan la hora local para que coincidan: antes el archivo llevaba la fecha
// local y las entradas iban en UTC, asi que no cuadraban.
func (l *Logger) rotate(now time.Time) error {
	day := now.Format("2006-01-02")
	if l.file != nil && l.day == day {
		return nil
	}
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
	f, err := os.OpenFile(filepath.Join(l.dir, day+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l.file, l.day = f, day
	l.purge(now)
	return nil
}

func (l *Logger) purge(now time.Time) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	limit := now.AddDate(0, 0, -RetentionDays)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", strings.TrimSuffix(name, ".jsonl"), now.Location())
		if err != nil || !day.Before(limit) {
			continue
		}
		_ = os.Remove(filepath.Join(l.dir, name))
	}
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
