// Alta y baja de los origenes web autorizados, desde el panel.
package api

import (
	"net/http"
	"net/url"
	"sort"
	"strings"

	"collatech-agent/internal/config"
)

// maxOrigenes acota la lista para que no crezca sin freno por descuido.
const maxOrigenes = 50

// listCORS devuelve los origenes permitidos y los rechazados hasta ahora.
//
// Los rechazados son lo util de verdad: el agente los va anotando, asi que
// quien no consigue conectar ve aqui su origen exacto y lo autoriza sin
// tener que adivinar como lo escribe su navegador.
func (s *Server) listCORS(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: map[string]any{
		"permitidos": s.origenesPermitidos(),
		"rechazados": s.OrigenesRechazados(),
	}})
}

// saveCORS guarda la lista. Solo responde en la PC del agente: autorizar
// origenes desde la red permitiria a quien estuviera dentro darse permiso
// a si mismo, que es justo lo que esta lista deberia impedir.
func (s *Server) saveCORS(w http.ResponseWriter, r *http.Request) {
	var lista []string
	if !s.decode(w, r, &lista) {
		return
	}
	limpia, err := normalizarOrigenes(lista)
	if err != nil {
		s.badRequest(w, err.Error())
		return
	}

	cfg := s.cfg
	cfg.AllowedCORS = limpia
	if err := config.Save(s.paths.Config, cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false,
			Error: "no se pudo guardar la configuracion: " + err.Error()})
		return
	}

	// Se aplica en caliente ademas de guardarse: pedirle a alguien que
	// reinicie el agente para que un origen empiece a valer es la clase de
	// paso que se olvida y hace pensar que no funciono.
	s.corsMu.Lock()
	s.cfg.AllowedCORS = limpia
	// Lo recien autorizado deja de estar "rechazado", para que no siga
	// saliendo en la lista de problemas.
	for _, o := range limpia {
		delete(s.corsVistos, o)
	}
	s.corsMu.Unlock()

	s.logger.Info("cors_actualizado", map[string]any{"permitidos": limpia})
	writeJSON(w, http.StatusOK, response{OK: true, Message: "origenes guardados", Data: limpia})
}

// origenesPermitidos lee la lista viva.
func (s *Server) origenesPermitidos() []string {
	s.corsMu.Lock()
	defer s.corsMu.Unlock()
	out := make([]string, len(s.cfg.AllowedCORS))
	copy(out, s.cfg.AllowedCORS)
	return out
}

// normalizarOrigenes valida y limpia lo que llega del panel.
//
// Un origen es esquema + host + puerto, sin ruta: "https://mi.app.com" o
// "http://192.168.1.20:4200". Se rechaza lo que no lo sea, porque una
// entrada mal escrita no da error en ningun sitio: simplemente no coincide
// nunca y el operador se queda sin saber por que su web sigue bloqueada.
func normalizarOrigenes(entradas []string) ([]string, error) {
	vistos := map[string]bool{}
	var out []string
	for _, e := range entradas {
		o := strings.TrimRight(strings.TrimSpace(e), "/")
		if o == "" {
			continue
		}
		if o == "*" {
			return nil, errComodin
		}
		u, err := url.Parse(o)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, errFormato(o)
		}
		if u.Path != "" || u.RawQuery != "" {
			return nil, errConRuta(o)
		}
		limpio := u.Scheme + "://" + u.Host
		if !vistos[limpio] {
			vistos[limpio] = true
			out = append(out, limpio)
		}
	}
	if len(out) > maxOrigenes {
		return nil, errDemasiados
	}
	sort.Strings(out)
	return out, nil
}

// Errores con explicacion, que es lo que lee quien se equivoca.
var (
	errComodin    = &erroresCORS{"el comodin \"*\" autorizaria a cualquier web de internet a imprimir y abrir el cajon. Pon los origenes concretos, por ejemplo https://mi-tienda.com"}
	errDemasiados = &erroresCORS{"demasiados origenes en la lista"}
)

func errFormato(o string) error {
	return &erroresCORS{"\"" + o + "\" no es un origen valido. Tiene que ser esquema, host y puerto, por ejemplo https://mi-tienda.com o http://192.168.1.20:4200"}
}

func errConRuta(o string) error {
	return &erroresCORS{"\"" + o + "\" lleva ruta. Un origen es solo esquema, host y puerto: quita todo lo que vaya despues"}
}

type erroresCORS struct{ msg string }

func (e *erroresCORS) Error() string { return e.msg }
