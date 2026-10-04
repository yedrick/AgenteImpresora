package config

import (
	"fmt"
	"net"
	"strings"
)

// RedDeConfianza es una IP o un rango que puede usar la API sin token.
type RedDeConfianza struct {
	Texto string
	Red   *net.IPNet
}

// ParseTrustedIPs convierte la lista de la configuracion en redes.
//
// Rechaza lo que en la practica seria apagar la autenticacion: un rango
// que abarque todo internet. Esta lista sirve para nombrar las maquinas de
// un local concreto; si hiciera falta abrirlo a cualquiera, eso es otra
// decision y no debe poder colarse por aqui disfrazada de "rango".
func ParseTrustedIPs(entradas []string) ([]RedDeConfianza, error) {
	var out []RedDeConfianza
	for _, e := range entradas {
		texto := strings.TrimSpace(e)
		if texto == "" {
			continue
		}

		// Una IP suelta vale como tal.
		if ip := net.ParseIP(texto); ip != nil {
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			out = append(out, RedDeConfianza{
				Texto: texto,
				Red:   &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)},
			})
			continue
		}

		_, red, err := net.ParseCIDR(texto)
		if err != nil {
			return nil, fmt.Errorf("trusted_ips: %q no es una IP ni un rango valido", texto)
		}
		ones, bits := red.Mask.Size()
		if ones == 0 {
			return nil, fmt.Errorf("trusted_ips: %q abarca toda la red y dejaria la API abierta a cualquiera. "+
				"Nombra las maquinas concretas, por ejemplo 192.168.1.20 o 192.168.1.0/24", texto)
		}
		if bits == 32 && ones < 8 {
			return nil, fmt.Errorf("trusted_ips: %q es demasiado amplio para una red local", texto)
		}
		out = append(out, RedDeConfianza{Texto: texto, Red: red})
	}
	return out, nil
}

// Contiene dice si la IP esta en alguna de las redes de confianza.
func Contiene(redes []RedDeConfianza, host string) bool {
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	for _, r := range redes {
		if r.Red.Contains(ip) {
			return true
		}
	}
	return false
}
