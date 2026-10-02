// Control de acceso y cabeceras: recuperacion de panicos, CORS, restriccion
// a la propia maquina y token para las peticiones que llegan por la red.
package api

import (
	"crypto/subtle"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
)

// adminPaths son endpoints que exponen configuracion, diagnostico del equipo
// o el historial: se atienden solo desde la propia PC del agente, pase lo que
// pase con allow_remote. /api/diagnostico llegaba a volcar la salida completa
// de netstat -ano a cualquiera en la red.
var adminPaths = map[string]bool{
	"/api/diagnostico": true,
	"/api/logs":        true,
	"/api/token":       true,
}

// adminWritePaths son cambios de configuracion: nunca desde la red.
var adminWritePaths = map[string]bool{
	"/api/settings":        true,
	"/api/printer-aliases": true,
	"/api/printers-config": true,
}

func (s *Server) isAdminRequest(r *http.Request) bool {
	if adminPaths[r.URL.Path] {
		return true
	}
	return r.Method == http.MethodPost && adminWritePaths[r.URL.Path]
}

// guard aplica el control de acceso a /api/*. Las paginas del panel y /health
// quedan fuera para que se puedan abrir desde un movil; las llamadas que esas
// paginas hacen si pasan por aqui.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		local := isLocalRequest(r)
		if s.isAdminRequest(r) && !local {
			writeJSON(w, http.StatusForbidden, response{OK: false,
				Error: "este endpoint solo esta disponible desde la PC donde corre el agente"})
			return
		}
		if !local && s.cfg.AuthToken != "" && !tokenMatches(r, s.cfg.AuthToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="CollaTech Agent"`)
			writeJSON(w, http.StatusUnauthorized, response{OK: false,
				Error: "token ausente o incorrecto: envia la cabecera 'Authorization: Bearer <token>'"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// tokenMatches compara en tiempo constante para no filtrar el token por el
// tiempo de respuesta.
func tokenMatches(r *http.Request, want string) bool {
	got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if got == "" {
		got = strings.TrimSpace(r.Header.Get("X-CollaTech-Token"))
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// recoverPanics evita que un payload malformado tumbe la conexion sin dejar
// rastro: sin esto, net/http aborta el socket y el cliente solo ve un EOF.
func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if s.logger != nil {
				s.logger.Error("panic_recovered", map[string]any{
					"error":  fmt.Sprint(rec),
					"method": r.Method,
					"path":   r.URL.Path,
					"stack":  string(debug.Stack()),
				})
			}
			writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: "internal error"})
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *Server) localhostOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocalRequest(r) {
			writeJSON(w, http.StatusForbidden, response{OK: false, Error: "local connections only"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && s.originAllowed(origin) {
			if hasWildcard(s.cfg.AllowedCORS) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// originAllowed compara el origen completo. Antes usaba HasPrefix, asi que
// con "http://localhost" en la lista tambien pasaba
// "http://localhost.atacante.com".
func (s *Server) originAllowed(origin string) bool {
	for _, allowed := range s.cfg.AllowedCORS {
		if allowed == "*" || strings.EqualFold(strings.TrimRight(allowed, "/"), strings.TrimRight(origin, "/")) {
			return true
		}
	}
	return false
}

func hasWildcard(allowed []string) bool {
	for _, item := range allowed {
		if item == "*" {
			return true
		}
	}
	return false
}
