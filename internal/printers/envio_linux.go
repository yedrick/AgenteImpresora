//go:build linux

package printers

import (
	"os"

	"golang.org/x/sys/unix"
)

// esperarEnvio espera a que el dispositivo haya entregado lo escrito.
//
// Go abre en modo no bloqueante todo dispositivo que admita poll, y
// /dev/usb/lp* lo admite. En ese modo usblp devuelve el write en cuanto pone
// los datos en el USB, sin esperar a que salgan, y al cerrar cancela lo que
// quede pendiente. Un ticket cabe entero en un solo envio, asi que se perdia
// completo: el trabajo salia "Completed" en menos de un milisegundo y la
// impresora no recibia nada, ni siquiera el corte.
//
// usblp vuelve a dar POLLOUT cuando termino de enviar. La espera respeta el
// deadline de escritura, asi que una impresora que no acepta datos acaba en
// error en vez de colgar al worker.
func esperarEnvio(f *os.File) error {
	rc, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var errPoll error
	errEspera := rc.Write(func(fd uintptr) bool {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
		for {
			n, err := unix.Poll(fds, 0)
			if err == unix.EINTR {
				continue
			}
			if err != nil {
				errPoll = err
				return true
			}
			// POLLERR o POLLHUP: el dispositivo se fue. No hay nada que
			// esperar; el error, si lo hay, sale al cerrar o en el proximo
			// trabajo.
			return n > 0 && fds[0].Revents&(unix.POLLOUT|unix.POLLERR|unix.POLLHUP) != 0
		}
	})
	if errEspera != nil {
		return errEspera
	}
	return errPoll
}
