package api

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestQRAdmiteLasDosFormas: el bloque maquetado escribe el QR como cadena y
// la linea de ticket como objeto. Quien copia un ejemplo de un sitio al otro
// usaba la forma que no era y la linea desaparecia del ticket en silencio.
func TestQRAdmiteLasDosFormas(t *testing.T) {
	casos := []struct {
		nombre string
		json   string
		data   string
		ec     string
	}{
		{"cadena suelta", `{"type":"qr","qr":"https://pago/A-1"}`, "https://pago/A-1", ""},
		{"objeto", `{"type":"qr","qr":{"data":"https://pago/A-1","ec":"H","size":6}}`, "https://pago/A-1", "H"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var l ticketLine
			if err := json.Unmarshal([]byte(c.json), &l); err != nil {
				t.Fatalf("no se pudo leer: %v", err)
			}
			if l.QR == nil {
				t.Fatal("el QR quedo vacio")
			}
			if l.QR.Data != c.data {
				t.Errorf("contenido %q, se esperaba %q", l.QR.Data, c.data)
			}
			if l.QR.EC != c.ec {
				t.Errorf("correccion %q, se esperaba %q", l.QR.EC, c.ec)
			}
		})
	}
}

func TestBarcodeAdmiteLasDosFormas(t *testing.T) {
	for _, c := range []struct{ nombre, j, data string }{
		{"cadena suelta", `{"type":"barcode","barcode":"7501234567890"}`, "7501234567890"},
		{"objeto", `{"type":"barcode","barcode":{"data":"7501234567890","type":"ean13"}}`, "7501234567890"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			var l ticketLine
			if err := json.Unmarshal([]byte(c.j), &l); err != nil {
				t.Fatalf("no se pudo leer: %v", err)
			}
			if l.Barcode == nil || l.Barcode.Data != c.data {
				t.Fatalf("contenido %+v, se esperaba %q", l.Barcode, c.data)
			}
		})
	}
}

// TestLineaVaciaAvisa: un QR sin contenido tiene que dar error, no salir un
// ticket al que le falta el QR de pago sin decir nada.
func TestLineaVaciaAvisa(t *testing.T) {
	casos := []struct{ nombre, j, esperado string }{
		{"qr sin contenido", `{"type":"qr"}`, "necesita el contenido"},
		{"qr vacio", `{"type":"qr","qr":""}`, "esta vacio"},
		{"barcode sin contenido", `{"type":"barcode"}`, "necesita el contenido"},
		{"barcode vacio", `{"type":"barcode","barcode":{"data":""}}`, "esta vacio"},
		{"layout sin bloque", `{"type":"layout"}`, "necesita el bloque"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			var l ticketLine
			if err := json.Unmarshal([]byte(c.j), &l); err != nil {
				t.Fatalf("no se pudo leer: %v", err)
			}
			err := applyLine(nil, l, 48, false, resolved{})
			if err == nil {
				t.Fatal("no dio error: la linea se perderia sin avisar")
			}
			if !strings.Contains(err.Error(), c.esperado) {
				t.Errorf("el mensaje fue %q y no menciona %q", err, c.esperado)
			}
		})
	}
}
