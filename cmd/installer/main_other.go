//go:build !windows

package main

import "fmt"

// El instalador registra un servicio, toca el registro y abre el firewall de
// Windows. Existe este stub para que `go build ./...` y `go vet ./...`
// funcionen en Linux/macOS al desarrollar y testear.
func main() {
	fmt.Println("El instalador de CollaTech Agent solo funciona en Windows.")
}
