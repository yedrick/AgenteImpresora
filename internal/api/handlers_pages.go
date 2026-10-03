// Paginas HTML del panel, servidas desde el binario.
package api

import (
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"strings"
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

func (s *Server) impresoras(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/impresoras.html")
}

// docUbuntu sirve la guia de Ubuntu. Se guarda en Markdown porque la misma
// guia se entrega como archivo en docs/, y la pagina la convierte en el
// navegador con un conversor propio de treinta lineas. No se carga ninguna
// libreria de fuera a proposito: la PC de una caja no suele tener internet,
// y una pagina que depende de un CDN ahi no se ve.
func (s *Server) docUbuntu(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/docs.html")
}

// docFuente entrega el Markdown crudo, que es lo que pinta la pagina.
func (s *Server) docFuente(w http.ResponseWriter, r *http.Request) {
	b, err := webFS.ReadFile("web/docs-ubuntu.md")
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{OK: false, Error: "la guia no esta en este binario"})
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write(b)
}

func (s *Server) sdk(w http.ResponseWriter, r *http.Request) {
	s.serveHTML(w, "web/sdk.html")
}

// sdkPaquete entrega el SDK de TypeScript, que va dentro del binario.
//
// Se incrusta a proposito: una caja de tienda suele no tener salida a
// internet, asi que "npm install collatech-sdk" no es una opcion ahi. Asi
// se instala desde el propio agente, que es lo unico que seguro alcanza.
func (s *Server) sdkPaquete(w http.ResponseWriter, r *http.Request) {
	b, err := webFS.ReadFile("web/sdk/collatech-sdk.tgz")
	if err != nil {
		writeJSON(w, http.StatusNotFound, response{OK: false, Error: "el paquete del SDK no esta en este binario"})
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="collatech-sdk.tgz"`)
	w.Write(b)
}

// serveAsset sirve el CSS y el JS compartidos por las tres paginas. Van
// aparte y no incrustados en cada HTML para no tener tres copias del tema
// que se desincronizan, que es justo lo que pasaba antes.
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	nombre := strings.TrimPrefix(r.URL.Path, "/")
	tipo := "text/css; charset=utf-8"
	if strings.HasSuffix(nombre, ".js") {
		tipo = "text/javascript; charset=utf-8"
	}
	b, err := webFS.ReadFile("web/" + nombre)
	if err != nil {
		b, err = os.ReadFile("web/" + nombre)
		if err != nil {
			writeJSON(w, http.StatusNotFound, response{OK: false, Error: nombre + " not found"})
			return
		}
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
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
