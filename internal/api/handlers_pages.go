// Paginas HTML del panel, servidas desde el binario.
package api

import (
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
)

func (s *Server) panel(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/index.html")
}

func (s *Server) designer(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/designer.html")
}

func (s *Server) diagnostico(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/diagnostico.html")
}

func (s *Server) serveHTML(w http.ResponseWriter, path string) {
	b, err := webFS.ReadFile(path)
	if err != nil {
		b, err = os.ReadFile(path) // fallback a disco
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{OK: false, Error: path + " not found"})
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Message: "healthy"})
}
