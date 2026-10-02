// Informe de diagnostico y lectura del registro, pensados para soporte.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"collatech-agent/internal/config"
)

func (s *Server) diagnosticReport(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	port := s.cfg.Port
	if port == 0 {
		port = config.DefaultPort
	}
	scheme := "http"
	if s.cfg.TLS.Enabled {
		scheme = "https"
	}
	ips := localIPv4s()
	localURL := fmt.Sprintf("%s://127.0.0.1:%d/health", scheme, port)
	lanChecks := []map[string]any{}
	panelURLs := []string{fmt.Sprintf("%s://127.0.0.1:%d/panel", scheme, port)}
	if host != "" {
		panelURLs = append(panelURLs, fmt.Sprintf("%s://%s:%d/panel", scheme, host, port))
	}
	for _, ip := range ips {
		healthURL := fmt.Sprintf("%s://%s:%d/health", scheme, ip, port)
		panelURLs = append(panelURLs, fmt.Sprintf("%s://%s:%d/panel", scheme, ip, port))
		lanChecks = append(lanChecks, map[string]any{
			"ip":     ip,
			"url":    healthURL,
			"result": probeURL(healthURL),
		})
	}

	configRaw := readTextFile(s.paths.Config, 64*1024)
	report := map[string]any{
		"generated_at": time.Now().Format(time.RFC3339),
		"service":      "CollaTech Agent",
		"runtime": map[string]any{
			"os":          runtime.GOOS,
			"arch":        runtime.GOARCH,
			"pid":         os.Getpid(),
			"executable":  exe,
			"working_dir": cwd,
		},
		"server": map[string]any{
			"scheme":       scheme,
			"host":         s.cfg.Host,
			"port":         port,
			"listen_addr":  fmt.Sprintf("%s:%d", s.cfg.Host, port),
			"allow_remote": s.cfg.AllowRemote,
			"tls_enabled":  s.cfg.TLS.Enabled,
			"panel_urls":   panelURLs,
		},
		"network": map[string]any{
			"hostname":        host,
			"local_ipv4":      ips,
			"localhost_check": probeURL(localURL),
			"lan_checks":      lanChecks,
		},
		"firewall": firewallDiagnostics(port, exe),
		"config": map[string]any{
			"path":   s.paths.Config,
			"active": redactConfig(s.cfg),
			"raw":    redactRawConfig(configRaw),
		},
		"advice": diagnosticAdvice(s.cfg, ips),
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: report})
}

func localIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	sort.Strings(out)
	return out
}

func probeURL(url string) map[string]any {
	start := time.Now()
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	elapsed := time.Since(start).Milliseconds()
	out := map[string]any{
		"url": url,
		"ok":  false,
		"ms":  elapsed,
	}
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer resp.Body.Close()
	out["status"] = resp.StatusCode
	out["ok"] = resp.StatusCode >= 200 && resp.StatusCode < 300
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	out["body"] = strings.TrimSpace(string(body))
	return out
}

func firewallDiagnostics(port int, exe string) map[string]any {
	return map[string]any{
		"os":           runtime.GOOS,
		"port_rule":    runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=GOServer"+strconv.Itoa(port)),
		"legacy_rule":  runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=CollaTech Agent 18743"),
		"program_rule": runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=CollaTech Agent App"),
		"listen":       runDiagnosticCommand(4*time.Second, "netstat", "-ano", "-p", "tcp"),
		"exe":          exe,
	}
}

func runDiagnosticCommand(timeout time.Duration, name string, args ...string) map[string]any {
	if runtime.GOOS != "windows" && (strings.EqualFold(name, "netsh") || strings.EqualFold(name, "netstat")) {
		return map[string]any{"ok": false, "error": "command only available on Windows"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if len(text) > 20000 {
		text = text[:20000] + "\n...[truncated]"
	}
	result := map[string]any{
		"command": strings.TrimSpace(name + " " + strings.Join(args, " ")),
		"ok":      err == nil,
		"output":  text,
	}
	if ctx.Err() == context.DeadlineExceeded {
		result["error"] = "timeout"
		return result
	}
	if err != nil {
		result["error"] = err.Error()
	}
	return result
}

func readTextFile(path string, max int64) map[string]any {
	out := map[string]any{"path": path, "ok": false}
	f, err := os.Open(path)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max))
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	out["ok"] = true
	out["text"] = string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}))
	return out
}

// redactConfig tapa el token antes de incluir la configuracion en el informe
// de diagnostico, que esta pensado para enviarse a soporte.
func redactConfig(cfg config.Config) config.Config {
	if cfg.AuthToken != "" {
		cfg.AuthToken = "***"
	}
	return cfg
}

var authTokenRe = regexp.MustCompile(`("auth_token"\s*:\s*)"[^"]*"`)

func redactRawConfig(raw map[string]any) map[string]any {
	text, ok := raw["text"].(string)
	if !ok {
		return raw
	}
	raw["text"] = authTokenRe.ReplaceAllString(text, `${1}"***"`)
	return raw
}

func diagnosticAdvice(cfg config.Config, ips []string) []string {
	var advice []string
	if cfg.Host == "127.0.0.1" || strings.EqualFold(cfg.Host, "localhost") {
		advice = append(advice, "El host esta en modo local. Para acceso desde celular usa host 0.0.0.0.")
	}
	if !cfg.AllowRemote {
		advice = append(advice, "allow_remote esta desactivado. Las peticiones externas seran bloqueadas por el agente.")
	}
	if cfg.AllowRemote && cfg.AuthToken == "" {
		advice = append(advice, "El acceso desde la red esta abierto SIN token. Cualquier equipo de la red puede imprimir. Reinicia el agente para que genere uno.")
	}
	if cfg.TLS.Enabled {
		advice = append(advice, "TLS/HTTPS esta activado. Debes probar con https:// y un certificado valido o desactivar TLS para red local.")
	}
	if len(ips) == 0 {
		advice = append(advice, "No se detecto una IPv4 LAN activa. Revisa WiFi/Ethernet.")
	}
	if len(advice) == 0 {
		advice = append(advice, "La configuracion del agente parece apta para LAN. Si el celular no entra, revisa firewall externo, antivirus o aislamiento WiFi del router.")
	}
	return advice
}

// maxLogTailBytes acota cuanto se lee de cada archivo de log. Antes se leia
// entero, asi que un GET /api/logs?limit=1 podia cargar cientos de MB.
const maxLogTailBytes = 1 << 20

// readTail devuelve como mucho los ultimos max bytes del archivo, recortados
// al primer salto de linea para no empezar a mitad de un registro.
func readTail(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size <= max {
		return io.ReadAll(f)
	}
	if _, err := f.Seek(size-max, io.SeekStart); err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if nl := bytes.IndexByte(b, '\n'); nl >= 0 {
		b = b[nl+1:]
	}
	return b, nil
}

func tailLogLines(dir string, limit int) ([]json.RawMessage, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []json.RawMessage{}, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var lines []json.RawMessage
	for i := len(names) - 1; i >= 0 && len(lines) < limit; i-- {
		b, err := readTail(filepath.Join(dir, names[i]), maxLogTailBytes)
		if err != nil {
			return nil, err
		}
		fileLines := strings.Split(strings.TrimSpace(string(b)), "\n")
		for j := len(fileLines) - 1; j >= 0 && len(lines) < limit; j-- {
			line := strings.TrimSpace(fileLines[j])
			if line != "" {
				lines = append(lines, json.RawMessage(line))
			}
		}
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines, nil
}
