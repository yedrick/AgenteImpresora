package escpos

import (
	"fmt"
	"image"
	"strings"

	"rsc.io/qr"
)

// maxQRImageModule es el lado maximo de cada punto, el mismo limite que el
// QR propio de la impresora.
const maxQRImageModule = 16

// QRImage dibuja un codigo QR como imagen. moduleSize es el lado de cada
// punto; con 0 se elige el mayor que quepa en maxWidth.
//
// Lo usan el bloque maquetado y QRWith cuando la impresora no tiene QR
// propio: muchas termicas baratas ignoran GS ( k sin avisar y el ticket sale
// sin el QR, pero todas las que imprimen imagenes lo sacan asi.
func QRImage(data string, moduleSize, maxWidth int, ec string) (*image.Gray, error) {
	if strings.TrimSpace(data) == "" {
		return nil, fmt.Errorf("el QR no tiene contenido")
	}
	nivel := qr.M
	switch strings.ToUpper(strings.TrimSpace(ec)) {
	case "L":
		nivel = qr.L
	case "Q":
		nivel = qr.Q
	case "H":
		nivel = qr.H
	}
	code, err := qr.Encode(data, nivel)
	if err != nil {
		return nil, fmt.Errorf("no se pudo generar el QR: %w", err)
	}

	// Zona de silencio: el estandar pide 4 modulos a cada lado o los
	// lectores fallan.
	const quiet = 4
	modulos := code.Size + 2*quiet

	if maxWidth <= 0 {
		maxWidth = 160
	}
	// El tamano se acota comparando por division: con moduleSize*modulos el
	// producto desbordaba el entero, la guarda quedaba en falso y se acababa
	// pidiendo una imagen de dimensiones absurdas o negativas.
	cabe := maxWidth / modulos
	if cabe < 1 {
		cabe = 1
	}
	if moduleSize <= 0 || moduleSize > cabe {
		moduleSize = cabe
	}
	if moduleSize > maxQRImageModule {
		moduleSize = maxQRImageModule
	}

	lado := modulos * moduleSize
	if lado > maxWidth {
		return nil, fmt.Errorf("el QR necesita %d puntos y solo hay %d: acorta el contenido o dale mas ancho", lado, maxWidth)
	}
	img := image.NewGray(image.Rect(0, 0, lado, lado))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			px := (x + quiet) * moduleSize
			py := (y + quiet) * moduleSize
			for dy := 0; dy < moduleSize; dy++ {
				fila := img.Pix[(py+dy)*img.Stride:]
				for dx := 0; dx < moduleSize; dx++ {
					fila[px+dx] = 0
				}
			}
		}
	}
	return img, nil
}
