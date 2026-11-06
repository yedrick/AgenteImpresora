// Package oyente cambia en caliente donde escucha el agente: solo esta PC
// (127.0.0.1) o toda la red (0.0.0.0), sin reiniciar el proceso.
//
// No se reinicia porque en Windows el servicio no vuelve a arrancar solo al
// salir, y porque reiniciar cortaria la cola de impresion.
package oyente

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
)

// Oyente es el socket de escucha de un http.Server que se puede cambiar.
type Oyente struct {
	mu     sync.Mutex
	srv    *http.Server
	ln     net.Listener
	host   string
	puerto int
	tlsCfg *tls.Config
}

// Abrir escucha en host:puerto y sirve srv en segundo plano. El socket se
// abre aqui mismo, de forma sincrona: un puerto ocupado sale como error y
// no como una linea perdida en el log.
func Abrir(srv *http.Server, host string, puerto int, tlsCfg *tls.Config) (*Oyente, error) {
	o := &Oyente{srv: srv, puerto: puerto, tlsCfg: tlsCfg}
	ln, err := o.escuchar(host)
	if err != nil {
		return nil, err
	}
	o.servir(ln, host)
	return o, nil
}

// Host dice donde escucha ahora.
func (o *Oyente) Host() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.host
}

// Reabrir pasa a escuchar en host. Cierra solo el socket que acepta
// conexiones, no las que ya estan abiertas: la peticion del panel que pidio
// el cambio recibe su respuesta. Si el host nuevo no se puede abrir, vuelve
// a escuchar donde estaba y lo dice en el error.
func (o *Oyente) Reabrir(host string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if host == o.host {
		return nil
	}
	anterior := o.host
	// Hay que cerrar antes de abrir: en Linux 0.0.0.0:P choca con
	// 127.0.0.1:P mientras este siga abierto.
	o.ln.Close()
	ln, err := o.escuchar(host)
	if err != nil {
		vuelta, err2 := o.escuchar(anterior)
		if err2 != nil {
			return fmt.Errorf("no se pudo escuchar en %s (%v) ni volver a %s: %w", host, err, anterior, err2)
		}
		o.servir(vuelta, anterior)
		return fmt.Errorf("no se pudo escuchar en %s: %v; sigue en %s", host, err, anterior)
	}
	o.servir(ln, host)
	return nil
}

func (o *Oyente) escuchar(host string) (net.Listener, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(o.puerto))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if o.tlsCfg != nil {
		ln = tls.NewListener(ln, o.tlsCfg)
	}
	return ln, nil
}

// servir se llama con o.mu tomado (o antes de que nadie mas vea o).
func (o *Oyente) servir(ln net.Listener, host string) {
	o.ln, o.host = ln, host
	// Serve devuelve un error al cerrarse el socket en Reabrir o en
	// Shutdown: es lo esperado, no hay nada que avisar.
	go o.srv.Serve(ln)
}
