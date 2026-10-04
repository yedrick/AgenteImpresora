package config

import "testing"

// TestLaListaDeConfianzaNoEsUnInterruptor: esta lista existe para nombrar
// las maquinas de un local mientras se migra un cliente antiguo. Un rango
// que lo abarque todo seria apagar la autenticacion por la puerta de atras,
// y eso tiene que ser una decision aparte, no colarse disfrazado de rango.
func TestLaListaDeConfianzaNoEsUnInterruptor(t *testing.T) {
	for _, malo := range []string{"0.0.0.0/0", "::/0", "1.0.0.0/1", "0.0.0.0/4"} {
		if _, err := ParseTrustedIPs([]string{malo}); err == nil {
			t.Errorf("se acepto %q, que deja la API abierta a cualquiera", malo)
		}
	}
}

func TestQuienEntraYQuienNo(t *testing.T) {
	redes, err := ParseTrustedIPs([]string{"192.168.1.20", "10.0.5.0/24", " 192.168.2.0/24 "})
	if err != nil {
		t.Fatalf("no se pudo leer una lista valida: %v", err)
	}

	for _, dentro := range []string{"192.168.1.20", "10.0.5.1", "10.0.5.254", "192.168.2.77"} {
		if !Contiene(redes, dentro) {
			t.Errorf("%s deberia entrar y no entra", dentro)
		}
	}
	for _, fuera := range []string{"192.168.1.21", "10.0.6.1", "8.8.8.8", "192.168.3.1", "", "no-es-una-ip"} {
		if Contiene(redes, fuera) {
			t.Errorf("%s NO deberia entrar y entra", fuera)
		}
	}
}

func TestEntradaInvalidaSeAvisa(t *testing.T) {
	if _, err := ParseTrustedIPs([]string{"la-pc-de-juan"}); err == nil {
		t.Error("se acepto un nombre de maquina: solo valen IPs y rangos")
	}
	// Lineas vacias se ignoran sin montar un drama.
	redes, err := ParseTrustedIPs([]string{"", "  ", "192.168.1.5"})
	if err != nil || len(redes) != 1 {
		t.Errorf("las lineas vacias deberian ignorarse: %v, %d redes", err, len(redes))
	}
}
