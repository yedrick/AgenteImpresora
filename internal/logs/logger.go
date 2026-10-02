package logs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	mu   sync.Mutex
	file *os.File
}

type Entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Event   string         `json:"event"`
	Details map[string]any `json:"details,omitempty"`
}

func NewJSONLogger(dir string) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := time.Now().Format("2006-01-02") + ".jsonl"
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &Logger{file: f}, nil
}

func (l *Logger) Info(event string, details map[string]any)  { l.write("info", event, details) }
func (l *Logger) Error(event string, details map[string]any) { l.write("error", event, details) }

func (l *Logger) write(level, event string, details map[string]any) {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = json.NewEncoder(l.file).Encode(Entry{
		Time:    time.Now().UTC().Format(time.RFC3339Nano),
		Level:   level,
		Event:   event,
		Details: details,
	})
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}
