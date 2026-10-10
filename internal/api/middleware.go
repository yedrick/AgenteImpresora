// Control de acceso y cabeceras: recuperacion de panicos, CORS, restriccion
// a la propia maquina y token para las peticiones que llegan por la red.
package api

import (
	"collatech-agent/internal/config"
	"crypto/subtle"
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
)

// adminPaths son endpoints que exponen configuracion, diagnostico del equipo
// o el historial: se atienden solo desde la propia PC del agente, pase lo que
// pase con allow_remote. /api/diagnostico llegaba a volcar la salida completa
// de netstat -ano a cualquiera en la red.
// adminPaths son las rutas que solo responden en la PC donde corre el
// agente, pase lo que pase con el token o las IPs de confianza.
//
// /api/network NO esta aqui a proposito. Devuelve el nombre del equipo y
// las URLs para alcanzarlo, que es justo lo que quiere ver quien abre el
// panel desde otra PC. Bloquearlo no protegia nada —quien pregunta ya esta
// conectado al agente y conoce al menos una de sus direcciones— y dejaba la
// seccion "Acceso en red" mostrando un error en bruto.
//
// Lo que si se queda: el diagnostico (configuracion completa, netstat,
// cortafuegos), los registros, el token y el paquete de soporte.
var adminPaths = map[string]bool{
	// Autorizar origenes desde la red permitiria a quien ya estuviera
	// dentro darse permiso a si mismo, que es justo lo que esta lista
	// deberia impedir.
	"/api/cors":           true,
	"/api/diagnostico":    true,
	"/api/logs":           true,
	"/api/token":          true,
	"/api/support-bundle": true,
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

// requireJSON exige Content-Type: application/json en todo POST.
//
// No es cosmetico: un POST con text/plain es una "peticion simple" para el
// navegador, asi que no dispara comprobacion previa y CORS no lo frena. Sin
// esto, cualquier web que abriera el cajero podia imprimir y abrir el cajon
// de dinero con un fetch de tres lineas. Pedir JSON obliga al navegador a
// preguntar antes, y ahi si se aplica la lista de origenes.
func (s *Server) requireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			next.ServeHTTP(w, r)
			return
		}
		tipo := r.Header.Get("Content-Type")
		if i := strings.IndexByte(tipo, ';'); i >= 0 {
			tipo = tipo[:i]
		}
		if !strings.EqualFold(strings.TrimSpace(tipo), "application/json") {
			writeJSON(w, http.StatusUnsupportedMediaType, response{OK: false,
				Error: "las peticiones POST deben enviar 'Content-Type: application/json'"})
			return
		}
		next.ServeHTTP(w, r)
	})
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
		// Una maquina de la lista de confianza entra sin token. Es para
		// convivir con clientes ya desplegados que no saben mandarlo.
		// Ojo: esto NO abre las rutas de administracion, que arriba ya se
		// limitaron a la propia PC del agente.
		if !local && s.enConfianza(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !local && s.cfg.AuthToken != "" && !tokenMatches(r, s.cfg.AuthToken) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="CollaTech Agent"`)
			// El mensaje decia solo "manda la cabecera Authorization", sin
			// decir de donde sacar el token ni que abrir la URL en el
			// navegador nunca va a funcionar, porque desde la barra de
			// direcciones no se pueden poner cabeceras. Quien llegaba aqui
			// se quedaba sin saber que hacer.
			writeJSON(w, http.StatusUnauthorized, response{OK: false,
				Error: "falta el token de acceso. Mandalo en la cabecera " +
					"'Authorization: Bearer <token>' o 'X-CollaTech-Token: <token>'. " +
					"Lo encuentras en el panel del agente, pestana Estado, abierto en su propia PC " +
					"(http://localhost:18743/panel). Abrir esta URL en el navegador no funciona: " +
					"desde la barra de direcciones no se pueden mandar cabeceras; usa el panel, " +
					"el SDK o curl -H."})
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
		if origin != "" && !s.originAllowed(origin) {
			// Sin esto, el rechazo era invisible: el agente respondia 200,
			// el navegador se tragaba la respuesta por falta de cabecera y
			// en el registro no quedaba nada que mirar.
			s.avisarOrigenRechazado(origin)
		}
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
	// "null" nunca es un origen legitimo para un agente local: lo manda
	// cualquier iframe con sandbox, y aceptarlo permitia que una web
	// cualquiera leyera la respuesta de /api/token.
	if strings.EqualFold(strings.TrimSpace(origin), "null") {
		return false
	}
	pedido := normalizarOrigen(origin)
	for _, allowed := range s.cfg.AllowedCORS {
		if allowed == "*" {
			return true
		}
		if normalizarOrigen(allowed) == pedido {
			return true
		}
	}
	return false
}

// normalizarOrigen deja el origen listo para comparar.
//
// Trata localhost, 127.0.0.1 y [::1] como lo mismo, porque lo son: las tres
// apuntan a esta misma maquina. Para el navegador si son origenes distintos,
// y esa diferencia era la trampa mas facil de pisar: con
// "http://localhost:4200" permitido, una aplicacion servida en
// "http://127.0.0.1:4200" quedaba bloqueada sin ninguna explicacion.
//
// No afloja la seguridad: quien pueda servir desde el bucle local de esta
// PC ya esta ejecutando codigo en ella.
func normalizarOrigen(origen string) string {
	o := strings.ToLower(strings.TrimRight(strings.TrimSpace(origen), "/"))
	for _, equivalente := range []string{"127.0.0.1", "[::1]", "::1"} {
		o = strings.Replace(o, "//"+equivalente, "//localhost", 1)
	}
	return o
}

func hasWildcard(allowed []string) bool {
	for _, item := range allowed {
		if item == "*" {
			return true
		}
	}
	return false
}

// avisarOrigenRechazado deja constancia de un origen bloqueado, una sola vez
// por origen, para no llenar el registro cuando una pagina reintenta.
func (s *Server) avisarOrigenRechazado(origin string) {
	s.corsMu.Lock()
	if s.corsVistos == nil {
		s.corsVistos = map[string]bool{}
	}
	nuevo := !s.corsVistos[origin]
	s.corsVistos[origin] = true
	s.corsMu.Unlock()
	if !nuevo {
		return
	}
	s.logger.Error("cors_rechazado", map[string]any{
		"origen":          origin,
		"permitidos":      s.cfg.AllowedCORS,
		"como_arreglarlo": "anade " + origin + " a allowed_cors en configs/config.json y reinicia el agente",
	})
}

// OrigenesRechazados devuelve los origenes bloqueados desde que arranco, para
// que el diagnostico pueda ensenarlos.
func (s *Server) OrigenesRechazados() []string {
	s.corsMu.Lock()
	defer s.corsMu.Unlock()
	out := make([]string, 0, len(s.corsVistos))
	for o := range s.corsVistos {
		out = append(out, o)
	}
	sort.Strings(out)
	return out
}

// enConfianza dice si la peticion viene de una maquina de la lista.
func (s *Server) enConfianza(r *http.Request) bool {
	if len(s.confianza) == 0 {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return config.Contiene(s.confianza, host)
}
