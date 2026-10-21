//go:build !linux

package printers

import "os"

// esperarEnvio no hace nada fuera de Linux: el problema de usblp, que
// descarta lo pendiente al cerrar, es propio de ese driver.
func esperarEnvio(f *os.File) error { return nil }
