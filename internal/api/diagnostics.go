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
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.diagnosticData()})
}

// diagnosticData arma el informe. Esta separado del manejador para poder
// incluirlo tambien en el paquete de soporte.
func (s *Server) diagnosticData() map[string]any {
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
	// Si el agente solo escucha en el bucle local, las IP de red NO van a
	// responder, y eso es lo correcto, no un fallo. Antes se probaban
	// igual y salian en rojo: parecia que algo estaba roto cuando estaba
	// bien configurado.
	soloLocal := s.cfg.Host == "" || s.cfg.Host == "127.0.0.1" || s.cfg.Host == "localhost" || s.cfg.Host == "::1"

	lanChecks := []map[string]any{}
	panelURLs := []string{fmt.Sprintf("%s://127.0.0.1:%d/panel", scheme, port)}
	if host != "" {
		panelURLs = append(panelURLs, fmt.Sprintf("%s://%s:%d/panel", scheme, host, port))
	}
	for _, ip := range ips {
		healthURL := fmt.Sprintf("%s://%s:%d/health", scheme, ip, port)
		panelURLs = append(panelURLs, fmt.Sprintf("%s://%s:%d/panel", scheme, ip, port))
		if soloLocal {
			lanChecks = append(lanChecks, map[string]any{
				"ip":  ip,
				"url": healthURL,
				"result": map[string]any{
					"ok":      true,
					"skipped": true,
					"detail": "No se prueba: el agente escucha solo en esta PC (host " +
						s.cfg.Host + "). Es lo correcto si tu aplicacion corre aqui mismo. " +
						"Para abrirlo a la red, pon host 0.0.0.0 y allow_remote true.",
				},
			})
			continue
		}
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
		// Lo que se ha rechazado por CORS desde que arranco. Es el dato que
		// falta cuando una aplicacion "no conecta" aunque el agente
		// responda 200: el navegador descarta la respuesta por falta de
		// cabecera y no queda rastro en ningun sitio.
		"cors": map[string]any{
			"permitidos": s.cfg.AllowedCORS,
			"rechazados": s.OrigenesRechazados(),
			"nota": "localhost, 127.0.0.1 y [::1] cuentan como el mismo origen. " +
				"Si tu aplicacion aparece en 'rechazados', anade ese origen exacto " +
				"a allowed_cors en la configuracion y reinicia el agente.",
		},
		"advice": diagnosticAdvice(s.cfg, ips),
	}
	return report
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

// firewallDiagnostics mira el cortafuegos del sistema que toque.
//
// Antes solo sabia de netsh, el de Windows, y en Linux o macOS cada
// comprobacion salia como "REVISAR: command only available on Windows".
// Eso no es un aviso, es ruido: manda a revisar algo que ni existe en esa
// maquina y tapa los avisos que si importan.
func firewallDiagnostics(port int, exe string) map[string]any {
	base := map[string]any{"os": runtime.GOOS, "exe": exe}

	switch runtime.GOOS {
	case "windows":
		base["port_rule"] = runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=GOServer"+strconv.Itoa(port))
		base["legacy_rule"] = runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=CollaTech Agent 18743")
		base["program_rule"] = runDiagnosticCommand(4*time.Second, "netsh", "advfirewall", "firewall", "show", "rule", "name=CollaTech Agent App")
		base["listen"] = runDiagnosticCommand(4*time.Second, "netstat", "-ano", "-p", "tcp")
		base["como_abrir"] = `netsh advfirewall firewall add rule name="CollaTech Agent" dir=in action=allow protocol=TCP localport=` + strconv.Itoa(port)

	case "linux":
		// ufw es lo habitual en Ubuntu. "ufw status" exige root, y el
		// agente no corre como root salvo que se instale como servicio:
		// que no se pueda leer no es un fallo, asi que se dice y ya.
		ufw := runDiagnosticCommand(4*time.Second, "ufw", "status")
		if texto, _ := ufw["output"].(string); strings.Contains(strings.ToLower(texto), "root") {
			ufw = map[string]any{
				"ok":        true,
				"no_aplica": true,
				"detail": "Hay que ser root para consultarlo. Miralo tu con: sudo ufw status. " +
					"Si dice inactive, el cortafuegos no esta estorbando.",
			}
		}
		base["ufw"] = ufw
		base["listen"] = runDiagnosticCommand(4*time.Second, "ss", "-ltnp")
		base["como_abrir"] = "sudo ufw allow " + strconv.Itoa(port) + "/tcp"

	case "darwin":
		base["cortafuegos"] = runDiagnosticCommand(4*time.Second,
			"/usr/libexec/ApplicationFirewall/socketfilterfw", "--getglobalstate")
		base["listen"] = runDiagnosticCommand(4*time.Second, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN")
		base["como_abrir"] = "El cortafuegos de macOS pregunta la primera vez que el agente escucha en la red; acepta ahi."

	default:
		base["nota"] = "No hay comprobacion de cortafuegos para " + runtime.GOOS + "."
	}
	return base
}

func runDiagnosticCommand(timeout time.Duration, name string, args ...string) map[string]any {
	// Un comando que no esta instalado no es un fallo del agente: se dice
	// y punto, sin marcarlo para revisar.
	if _, err := exec.LookPath(name); err != nil {
		return map[string]any{
			"ok":        true,
			"no_aplica": true,
			"detail":    "en esta maquina no esta " + name,
		}
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

// redactTokenText tapa el token de un texto de configuracion.
//
// Se intenta primero analizando el JSON y volviendolo a escribir: una
// expresion regular sobre el texto crudo no veia un "auth_token" escrito con
// escapes, que para el analizador es el mismo campo, y el token se colaba en
// el informe. La expresion queda solo de respaldo por si el archivo no es
// JSON valido.
func redactTokenText(text string) string {
	var crudo map[string]any
	if err := json.Unmarshal(bytes.TrimPrefix([]byte(text), []byte{0xEF, 0xBB, 0xBF}), &crudo); err == nil {
		if _, ok := crudo["auth_token"]; ok {
			crudo["auth_token"] = "***"
		}
		if b, err := json.MarshalIndent(crudo, "", "  "); err == nil {
			return string(b)
		}
	}
	return authTokenRe.ReplaceAllString(text, `${1}"***"`)
}

func redactRawConfig(raw map[string]any) map[string]any {
	text, ok := raw["text"].(string)
	if !ok {
		return raw
	}
	raw["text"] = redactTokenText(text)
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
