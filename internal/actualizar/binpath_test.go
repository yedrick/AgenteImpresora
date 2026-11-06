package actualizar

import "testing"

func TestRutaDeBinPath(t *testing.T) {
	casos := map[string]string{
		`"C:\Program Files\CollaTech Agent\CollaTechAgent.exe"`:                                    `C:\Program Files\CollaTech Agent\CollaTechAgent.exe`,
		`"C:\Program Files\CollaTech Agent\CollaTechAgent.exe" --config "C:\x\config.json"`:        `C:\Program Files\CollaTech Agent\CollaTechAgent.exe`,
		`C:\Program Files\CollaTech Agent\CollaTechAgent.exe`:                                      `C:\Program Files\CollaTech Agent\CollaTechAgent.exe`,
		`D:\caja\CollaTechAgent-32bits.EXE --config "D:\caja\configs\config.json" --data-dir "D:"`: `D:\caja\CollaTechAgent-32bits.EXE`,
	}
	for bin, want := range casos {
		got, err := rutaDeBinPath(bin)
		if err != nil || got != want {
			t.Errorf("rutaDeBinPath(%q) = %q, %v; quiero %q", bin, got, err, want)
		}
	}
	if _, err := rutaDeBinPath("  "); err == nil {
		t.Error("un binPath vacio tiene que dar error")
	}
}
