//go:build !linux && !windows

package actualizar

import (
	"context"
	"errors"
)

var errNoSoportado = errors.New("en este sistema se actualiza con el paquete a mano")

type sistemaOtro struct{}

// SistemaActual devuelve el sistema de esta maquina. En macOS y otros, el
// boton no esta disponible.
func SistemaActual(string) Sistema { return sistemaOtro{} }

func (sistemaOtro) Soportado() (bool, string)       { return false, errNoSoportado.Error() }
func (sistemaOtro) RutaEjecutable() (string, error) { return "", errNoSoportado }
func (sistemaOtro) Parar(context.Context) error     { return errNoSoportado }
func (sistemaOtro) Arrancar(context.Context) error  { return errNoSoportado }
func (sistemaOtro) Lanzar(string) error             { return errNoSoportado }
