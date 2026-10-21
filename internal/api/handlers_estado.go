// Estado real de las estaciones y consulta de un trabajo concreto: lo que un
// backend necesita para saber, sin imprimir, si puede contar con una
// impresora y si su ticket salio.
package api

import (
	"net/http"
	"strings"
	"sync"

	"collatech-agent/internal/printers"
	"collatech-agent/internal/settings"
)

// EstadoEstacion es una estacion con el estado de cada impresora a la que
// imprime.
type EstadoEstacion struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Configured es false cuando el nombre no esta dado de alta y se
	// consulto como destino directo, igual que se imprimiria.
	Configured bool `json:"configured"`
	// Online y Ready valen para la estacion entera: con varias impresoras,
	// solo si lo estan todas.
	Online  bool              `json:"online"`
	Ready   bool              `json:"ready"`
	Targets []printers.Estado `json:"targets"`
}

// estadoDestino consulta una impresora sin pisar un trabajo en curso.
func (s *Server) estadoDestino(target string) printers.Estado {
	var e printers.Estado
	if !s.queue.SiLibre(target, func() { e = s.printers.Estado(target) }) {
		// Si esta imprimiendo, funciona.
		return printers.Estado{
			Target: target, Online: true, Ready: true, Printing: true,
			Paper:  printers.PaperUnknown,
			Detail: "imprimiendo un trabajo; no se la consulta para no interrumpirlo",
		}
	}
	return e
}

func (s *Server) estadoEstacion(p settings.Printer, configurada bool) EstadoEstacion {
	targets := p.Targets()
	est := EstadoEstacion{
		Name: p.Name, Description: p.Description, Configured: configurada,
		Targets: make([]printers.Estado, len(targets)),
	}
	// En paralelo: una impresora de red apagada tarda el timeout entero en
	// no contestar, y no tiene por que retrasar a las demas.
	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(i int, t string) {
			defer wg.Done()
			est.Targets[i] = s.estadoDestino(t)
		}(i, t)
	}
	wg.Wait()
	est.Online, est.Ready = len(targets) > 0, len(targets) > 0
	for _, e := range est.Targets {
		est.Online = est.Online && e.Online
		est.Ready = est.Ready && e.Ready
	}
	return est
}

// EstadoEstaciones consulta todas las estaciones dadas de alta. Lo usa
// tambien el conector con la nube para informar a Laravel.
func (s *Server) EstadoEstaciones() []EstadoEstacion {
	cfg, err := s.loadSettings()
	if err != nil {
		return []EstadoEstacion{}
	}
	out := make([]EstadoEstacion, len(cfg.Printers))
	var wg sync.WaitGroup
	for i, p := range cfg.Printers {
		wg.Add(1)
		go func(i int, p settings.Printer) {
			defer wg.Done()
			out[i] = s.estadoEstacion(p, true)
		}(i, p)
	}
	wg.Wait()
	return out
}

func (s *Server) listStations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true, Data: s.EstadoEstaciones()})
}

func (s *Server) getStation(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	if name == "" {
		s.badRequest(w, "falta el nombre de la estacion")
		return
	}
	cfg, _ := s.loadSettings()
	if p, ok := cfg.Find(name); ok {
		writeJSON(w, http.StatusOK, response{OK: true, Data: s.estadoEstacion(p, true)})
		return
	}
	// Un nombre que no esta dado de alta se imprime tal cual, asi que
	// tambien se consulta tal cual.
	writeJSON(w, http.StatusOK, response{OK: true,
		Data: s.estadoEstacion(settings.Printer{Name: name, Target: name}, false)})
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	job, ok := s.queue.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, response{OK: false,
			Error: "no hay ningun trabajo con id " + id + " (o es tan antiguo que ya se purgo)"})
		return
	}
	job.Payload = nil
	writeJSON(w, http.StatusOK, response{OK: true, Data: job})
}
