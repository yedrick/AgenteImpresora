package api

import "testing"

// Instalado como servicio, el agente corre con ProtectSystem=strict: /run es
// de solo lectura y "ufw status" revienta al crear /run/ufw.lock. Eso no es
// un problema del cortafuegos: el panel no puede marcarlo para revisar ni
// ensenar el traceback de Python.
func TestUfwSinPermisoParaLeerNoEsUnFallo(t *testing.T) {
	casos := map[string]string{
		"sin root":     "ERROR: You need to be root to run this script",
		"solo lectura": "Traceback (most recent call last):\n  File \"/usr/sbin/ufw\", line 130\nOSError: [Errno 30] Read-only file system: '/run/ufw.lock'",
	}
	for nombre, salida := range casos {
		r := resultadoUfw(map[string]any{"ok": false, "output": salida, "error": "exit status 1"})
		if r["ok"] != true || r["no_aplica"] != true {
			t.Errorf("%s: quiero ok y no_aplica, tengo %v", nombre, r)
		}
		if _, hay := r["output"]; hay {
			t.Errorf("%s: no se tiene que ensenar la salida de ufw: %v", nombre, r)
		}
	}
	bien := map[string]any{"ok": true, "output": "Status: inactive"}
	if r := resultadoUfw(bien); r["output"] != "Status: inactive" {
		t.Errorf("una lectura normal tiene que quedar igual, tengo %v", r)
	}
}
