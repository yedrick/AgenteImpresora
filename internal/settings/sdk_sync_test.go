package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// TestPrinterCoincideConElSDK comprueba que cada campo JSON de Printer existe
// tambien en la interfaz PrinterProfile del SDK de TypeScript.
//
// Hace falta porque las dos definiciones viven en lenguajes distintos y nada
// las ata: cuando se anadio "model" al agente, el SDK se quedo sin el y desde
// Angular no habia forma de declararlo, aunque el agente si lo guardaba. El
// fallo no lo vio ningun compilador.
func TestPrinterCoincideConElSDK(t *testing.T) {
	ruta := filepath.Join("..", "..", "collatech-sdk", "src", "types.ts")
	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Skipf("no esta el SDK en el arbol (%v): nada que comparar", err)
	}

	bloque := regexp.MustCompile(`(?s)export interface PrinterProfile \{(.*?)\n\}`).FindSubmatch(datos)
	if bloque == nil {
		t.Fatal("no se encontro la interfaz PrinterProfile en el SDK: la prueba quedaria sin comprobar nada")
	}
	// Los nombres de campo del SDK, ignorando comentarios.
	campoTS := regexp.MustCompile(`(?m)^\s{2}(\w+)\??:`)
	enSDK := map[string]bool{}
	for _, m := range campoTS.FindAllSubmatch(bloque[1], -1) {
		enSDK[string(m[1])] = true
	}
	if len(enSDK) == 0 {
		t.Fatal("no se leyo ningun campo del SDK: la expresion regular dejo de servir")
	}

	tipo := reflect.TypeOf(Printer{})
	var faltan []string
	for i := 0; i < tipo.NumField(); i++ {
		etiqueta := tipo.Field(i).Tag.Get("json")
		nombre := strings.Split(etiqueta, ",")[0]
		if nombre == "" || nombre == "-" {
			continue
		}
		if !enSDK[nombre] {
			faltan = append(faltan, nombre)
		}
	}
	if len(faltan) > 0 {
		t.Errorf("el agente guarda estos campos y el SDK no los declara: %v\n"+
			"anadelos a PrinterProfile en %s, o desde TypeScript no se pueden usar",
			faltan, ruta)
	}
	t.Logf("%d campos del agente, todos presentes en el SDK", tipo.NumField())
}
