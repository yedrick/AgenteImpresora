package actualizar

import (
	"context"
	"runtime"
)

// Plataforma es la clave del manifiesto para este ejecutable: el sistema y
// la arquitectura con que se compilo, no los de la maquina. Asi un agente de
// 32 bits en un Windows de 64 sigue siendo de 32.
func Plataforma() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// Sistema es lo que depende del sistema operativo.
type Sistema interface {
	// Soportado dice si aqui se puede actualizar con el boton y, si no, por que.
	Soportado() (bool, string)
	// RutaEjecutable es el ejecutable que arranca el servicio.
	RutaEjecutable() (string, error)
	Parar(ctx context.Context) error
	Arrancar(ctx context.Context) error
	// Lanzar pide aplicar la orden fuera del servicio, que se va a reiniciar.
	Lanzar(ordenRuta string) error
}
