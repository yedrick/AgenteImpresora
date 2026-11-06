package actualizar

import (
	"fmt"
	"strings"
)

// rutaDeBinPath saca el ejecutable de un binPath, con o sin comillas y con
// o sin argumentos detras.
func rutaDeBinPath(bin string) (string, error) {
	bin = strings.TrimSpace(bin)
	if strings.HasPrefix(bin, `"`) {
		if fin := strings.Index(bin[1:], `"`); fin >= 0 {
			return bin[1 : fin+1], nil
		}
	}
	if i := strings.Index(strings.ToLower(bin), ".exe"); i >= 0 {
		return bin[:i+4], nil
	}
	if bin == "" {
		return "", fmt.Errorf("el servicio no tiene ejecutable")
	}
	return bin, nil
}
