package printers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestArchivoNoSePisa comprueba que imprimir dos veces a un archivo normal
// deja los dos tickets, uno detras de otro.
//
// Apuntar a un archivo es la unica forma de probar el agente sin impresora.
// Antes se abria con O_WRONLY a secas, que escribe desde el byte 0: el
// segundo ticket pisaba al primero y, al ser mas corto, quedaba pegado a la
// cola del anterior. El trabajo se marcaba "Completed" igual, asi que nada
// avisaba de que faltaba un ticket.
func TestArchivoNoSePisa(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "rollo.bin")
	if err := os.WriteFile(ruta, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	primero := []byte("PRIMER TICKET, que es bastante largo para notar si lo pisan")
	segundo := []byte("SEGUNDO")

	for _, datos := range [][]byte{primero, segundo} {
		p := newDevicePrinter(ruta, "device")
		if err := p.Connect(); err != nil {
			t.Fatalf("no se pudo abrir el destino: %v", err)
		}
		if err := p.Print(datos); err != nil {
			t.Fatalf("no se pudo imprimir: %v", err)
		}
		if err := p.Disconnect(); err != nil {
			t.Fatal(err)
		}
	}

	salida, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	esperado := string(primero) + string(segundo)
	if string(salida) != esperado {
		t.Errorf("el archivo quedo con %q\nse esperaba   %q", salida, esperado)
	}
}

// TestDestinoQueNoExisteFalla: una ruta de dispositivo mal escrita tiene que
// dar error, no crear un archivo y aparentar que imprimio.
func TestDestinoQueNoExisteFalla(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "no", "existe", "lp0")
	p := newDevicePrinter(ruta, "device")
	if err := p.Connect(); err == nil {
		_ = p.Disconnect()
		t.Fatal("abrio un destino inexistente: una ruta mal escrita pareceria que imprime")
	}
}
