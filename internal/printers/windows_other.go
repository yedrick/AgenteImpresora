//go:build !windows

package printers

import "fmt"

// Implementacion para sistemas que no son Windows. Existe para que el resto
// del proyecto compile y se pueda testear fuera de Windows; imprimir por el
// spooler sigue siendo exclusivo de Windows.
type WindowsPrinter struct {
	name string
}

func NewWindowsPrinter(name string) *WindowsPrinter { return &WindowsPrinter{name: name} }

func (p *WindowsPrinter) Connect() error    { return nil }
func (p *WindowsPrinter) Disconnect() error { return nil }

func (p *WindowsPrinter) Print(data []byte) error {
	return fmt.Errorf("el spooler de Windows no esta disponible en este sistema (impresora %q); usa tcp://host:9100", p.name)
}

func (p *WindowsPrinter) Status() error {
	return fmt.Errorf("el spooler de Windows no esta disponible en este sistema")
}

func enumWindowsPrinters() []Info { return nil }
