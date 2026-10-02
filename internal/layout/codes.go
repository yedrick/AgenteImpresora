package layout

import (
	"fmt"
	"image"
	"strings"

	"rsc.io/qr"
)

// renderQR dibuja un codigo QR como imagen. moduleSize es el lado de cada
// punto; con 0 se elige el mayor que quepa en maxWidth.
func renderQR(data string, moduleSize, maxWidth int, ec string) (image.Image, error) {
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

	if moduleSize <= 0 {
		if maxWidth <= 0 {
			maxWidth = 160
		}
		moduleSize = maxWidth / modulos
	}
	if moduleSize < 1 {
		moduleSize = 1
	}
	if maxWidth > 0 && moduleSize*modulos > maxWidth {
		moduleSize = maxWidth / modulos
		if moduleSize < 1 {
			moduleSize = 1
		}
	}

	lado := modulos * moduleSize
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

// --- Code 128 ---------------------------------------------------------------

// code128Patterns son los anchos de barra y espacio de cada simbolo, segun el
// estandar. Cada digito es el ancho de un elemento, alternando barra y
// espacio empezando por barra.
var code128Patterns = []string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312",
	"132212", "221213", "221312", "231212", "112232", "122132", "122231", "113222",
	"123122", "123221", "223211", "221132", "221231", "213212", "223112", "312131",
	"311222", "321122", "321221", "312212", "322112", "322211", "212123", "212321",
	"232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
	"231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121",
	"313121", "211331", "231131", "213113", "213311", "213131", "311123", "311321",
	"331121", "312113", "312311", "332111", "314111", "221411", "431111", "111224",
	"111422", "121124", "121421", "141122", "141221", "112214", "112412", "122114",
	"122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
	"111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112",
	"421211", "212141", "214121", "412121", "111143", "111341", "131141", "114113",
	"114311", "411113", "411311", "113141", "114131", "311141", "411131", "211412",
	"211214", "211232", "2331112",
}

const (
	code128StartB = 104
	code128Stop   = 106
)

// encodeCode128B convierte el texto en la secuencia de simbolos del juego B,
// que cubre el ASCII imprimible: es el que usa un ticket.
func encodeCode128B(data string) ([]int, error) {
	if data == "" {
		return nil, fmt.Errorf("el codigo de barras no tiene contenido")
	}
	valores := []int{code128StartB}
	for _, r := range data {
		if r < 32 || r > 126 {
			return nil, fmt.Errorf("el caracter %q no se puede codificar en Code128", string(r))
		}
		valores = append(valores, int(r)-32)
	}
	// Digito de control: suma ponderada por la posicion, modulo 103.
	suma := code128StartB
	for i, v := range valores[1:] {
		suma += (i + 1) * v
	}
	valores = append(valores, suma%103, code128Stop)
	return valores, nil
}

// renderBarcode dibuja un Code128. moduleWidth es el grosor de la barra fina;
// con 0 se elige el mayor que quepa en maxWidth.
func renderBarcode(data string, height, moduleWidth, maxWidth int) (image.Image, error) {
	valores, err := encodeCode128B(data)
	if err != nil {
		return nil, err
	}
	// Ancho total en modulos, mas la zona de silencio de 10 a cada lado.
	const quiet = 10
	modulos := quiet * 2
	for _, v := range valores {
		for _, d := range code128Patterns[v] {
			modulos += int(d - '0')
		}
	}
	if moduleWidth <= 0 {
		if maxWidth <= 0 {
			maxWidth = 400
		}
		moduleWidth = maxWidth / modulos
	}
	if moduleWidth < 1 {
		moduleWidth = 1
	}
	if maxWidth > 0 && moduleWidth*modulos > maxWidth {
		moduleWidth = maxWidth / modulos
		if moduleWidth < 1 {
			moduleWidth = 1
		}
	}
	if height <= 0 {
		height = 70
	}

	ancho := modulos * moduleWidth
	img := image.NewGray(image.Rect(0, 0, ancho, height))
	for i := range img.Pix {
		img.Pix[i] = 255
	}

	x := quiet * moduleWidth
	for _, v := range valores {
		barra := true // cada patron empieza por barra
		for _, d := range code128Patterns[v] {
			w := int(d-'0') * moduleWidth
			if barra {
				for yy := 0; yy < height; yy++ {
					fila := img.Pix[yy*img.Stride:]
					for dx := 0; dx < w && x+dx < ancho; dx++ {
						fila[x+dx] = 0
					}
				}
			}
			x += w
			barra = !barra
		}
	}
	return img, nil
}
