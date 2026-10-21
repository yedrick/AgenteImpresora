//go:build !linux

package printers

func estadoDispositivoSO(path string) Estado { return estadoAbriendo(path) }

// usbConectado no sabe buscar por numero de serie fuera de Linux: se da por
// enchufada y manda lo que diga el spooler.
func usbConectado(serial string) (bool, string) { return true, "" }
