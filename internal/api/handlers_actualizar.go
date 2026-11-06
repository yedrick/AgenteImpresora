// Actualizar el agente desde el panel. Solo responde en la PC del agente:
// ver adminPaths en middleware.go.
package api

import (
	"context"
	"net/http"
	"time"

	"collatech-agent/internal/actualizar"
)

const sinActualizador = "este agente no se puede actualizar con el boton"

// estadoActualizacion es lo que el panel necesita para pintar la tarjeta.
func (s *Server) estadoActualizacion(w http.ResponseWriter, r *http.Request) {
	datos := map[string]any{
		"version_actual": s.Version,
		"plataforma":     actualizar.Plataforma(),
		"soportado":      false,
		"motivo":         sinActualizador,
	}
	if a := s.Actualizador; a != nil {
		ok, motivo := a.Soportado()
		datos["soportado"] = ok
		datos["motivo"] = motivo
		datos["estado"] = a.Estado()
		datos["ultimo_resultado"] = a.UltimoResultado()
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: datos})
}

func (s *Server) buscarActualizacion(w http.ResponseWriter, r *http.Request) {
	if s.Actualizador == nil {
		writeJSON(w, http.StatusServiceUnavailable, response{OK: false, Error: sinActualizador})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	d, err := s.Actualizador.Buscar(ctx)
	if err != nil {
		s.logger.Info("actualizacion_buscar_fallo", map[string]any{"error": err.Error()})
		writeJSON(w, http.StatusBadGateway, response{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Data: d})
}

func (s *Server) instalarActualizacion(w http.ResponseWriter, r *http.Request) {
	if s.Actualizador == nil {
		writeJSON(w, http.StatusServiceUnavailable, response{OK: false, Error: sinActualizador})
		return
	}
	var cuerpo struct {
		Version string `json:"version"`
	}
	if !s.decode(w, r, &cuerpo) {
		return
	}
	if err := s.Actualizador.Instalar(cuerpo.Version); err != nil {
		writeJSON(w, http.StatusConflict, response{OK: false, Error: err.Error()})
		return
	}
	s.logger.Info("actualizacion_iniciada", map[string]any{"desde": s.Version, "hasta": cuerpo.Version})
	writeJSON(w, http.StatusAccepted, response{OK: true, Message: "actualizacion en marcha"})
}
