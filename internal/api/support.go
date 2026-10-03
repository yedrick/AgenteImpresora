// Paquete de soporte: todo lo que hace falta para diagnosticar un problema,
// en un solo archivo que el cliente puede enviar.

package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxBundleLogDays son los dias de registro que se incluyen.
const maxBundleLogDays = 7

// supportBundle devuelve un .zip con el informe de diagnostico, los ultimos
// dias de registro, la configuracion y los ajustes, todo con el token tapado.
// Antes habia que explicarle al cliente por telefono que archivos buscar.
func (s *Server) supportBundle(w http.ResponseWriter, r *http.Request) {
	nombre := fmt.Sprintf("collatech-soporte-%s.zip", time.Now().Format("2006-01-02-1504"))

	// El zip se arma en memoria y se manda entero. Antes se escribian las
	// cabeceras y se iba emitiendo: si el informe tardaba (el diagnostico
	// lanza varios netsh con espera), el WriteTimeout cortaba el archivo a
	// medias y el operador recibia un zip corrupto presentado como exito.
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)

	add := func(path string, data []byte) {
		f, err := z.Create(path)
		if err != nil {
			return
		}
		_, _ = f.Write(data)
	}

	// Informe de diagnostico, el mismo de /api/diagnostico.
	if data, err := json.MarshalIndent(s.diagnosticData(), "", "  "); err == nil {
		add("diagnostico.json", data)
	}

	// Configuracion, sin el token.
	if raw, err := os.ReadFile(s.paths.Config); err == nil {
		add("config.json", []byte(redactTokenText(string(raw))))
	}

	// Ajustes e impresoras dadas de alta.
	if cfg, err := s.loadSettings(); err == nil {
		if data, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			add("settings.json", data)
		}
	}

	// Registro de los ultimos dias.
	for _, name := range recentLogFiles(s.paths.Logs, maxBundleLogDays) {
		if data, err := readTail(filepath.Join(s.paths.Logs, name), maxLogTailBytes); err == nil {
			add("logs/"+name, data)
		}
	}

	// Estado de la cola.
	if data, err := json.MarshalIndent(s.queue.List(), "", "  "); err == nil {
		add("cola.json", data)
	}

	add("LEEME.txt", []byte(bundleReadme(nombre)))

	if err := z.Close(); err != nil {
		s.logger.Error("support_bundle", map[string]any{"error": err.Error()})
		writeJSON(w, http.StatusInternalServerError, response{OK: false,
			Error: "no se pudo generar el paquete de soporte"})
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	_, _ = w.Write(buf.Bytes())
	s.logger.Info("support_bundle", map[string]any{"archivo": nombre, "bytes": buf.Len()})
}

// recentLogFiles devuelve los archivos de registro mas recientes.
func recentLogFiles(dir string, max int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) > max {
		names = names[:max]
	}
	return names
}

func bundleReadme(nombre string) string {
	return "CollaTech Agent - paquete de soporte\n" +
		"====================================\n\n" +
		"Generado: " + time.Now().Format("2006-01-02 15:04:05") + "\n" +
		"Archivo:  " + nombre + "\n\n" +
		"Contenido:\n" +
		"  diagnostico.json  equipo, red, firewall y configuracion activa\n" +
		"  config.json       configuracion del agente (el token va tapado)\n" +
		"  settings.json     impresoras dadas de alta y preferencias\n" +
		"  cola.json         trabajos recientes y su resultado\n" +
		"  logs/             registro de los ultimos " + fmt.Sprint(maxBundleLogDays) + " dias\n\n" +
		"El registro guarda la forma de cada trabajo (impresora, numero de\n" +
		"lineas, errores), nunca el contenido impreso: aqui no hay nombres de\n" +
		"clientes ni importes.\n"
}
