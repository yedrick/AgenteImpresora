package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Limite de peticiones por origen. No habia ninguno: un bucle mal escrito en
// el sistema del cliente, o cualquiera en la red, podia llenar la cola y
// gastar el rollo de papel entero.
const (
	// burstPorIP son las impresiones seguidas que se admiten de golpe.
	burstPorIP = 30
	// recargaPorSegundo es el ritmo sostenido que se permite despues.
	recargaPorSegundo = 10
	// olvidoDeClientes limpia los que llevan rato sin aparecer.
	olvidoDeClientes = 10 * time.Minute
)

type bucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*bucket
	burst   float64
	refill  float64
	lastGC  time.Time
}

func newRateLimiter(burst, refillPerSecond int) *rateLimiter {
	return &rateLimiter{
		clients: map[string]*bucket{},
		burst:   float64(burst),
		refill:  float64(refillPerSecond),
		lastGC:  time.Now(),
	}
}

// allow descuenta una peticion del origen y dice si se atiende.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastGC) > olvidoDeClientes {
		for k, b := range l.clients {
			if now.Sub(b.last) > olvidoDeClientes {
				delete(l.clients, k)
			}
		}
		l.lastGC = now
	}

	b, ok := l.clients[key]
	if !ok {
		l.clients[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	b.tokens += now.Sub(b.last).Seconds() * l.refill
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// rateLimit se aplica solo a /api/print/*: las consultas de estado las hace
// el panel cada pocos segundos y no tiene sentido frenarlas.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	limiter := newRateLimiter(burstPorIP, recargaPorSegundo)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !isPrintPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		if !limiter.allow(host, time.Now()) {
			s.logger.Error("rate_limited", map[string]any{"origen": host, "ruta": r.URL.Path})
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusTooManyRequests, response{OK: false,
				Error: "demasiadas impresiones seguidas desde este equipo; reintenta en un momento"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isPrintPath(path string) bool {
	const prefix = "/api/print/"
	return len(path) > len(prefix) && path[:len(prefix)] == prefix
}
