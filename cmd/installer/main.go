package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	_ "embed"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed CollaTechAgent.bin
var agentBinary []byte

const appTitle = "CollaTech Agent - Instalador"

type installResult struct {
	installDir string
	hostname   string
	ips        []string
	firewallOK bool
	serviceOK  bool
}

func main() {
	if runtime.GOOS != "windows" {
		return
	}

	if !isElevated() {
		if relaunchElevated() {
			return
		}
		messageBox(
			"CollaTech Agent necesita permisos de administrador para instalarse "+
				"(inicio automatico y apertura del puerto 18743 en el Firewall).\n\n"+
				"Vuelve a ejecutar el instalador y acepta el aviso de Windows.",
			appTitle, mbOK|mbIconError)
		return
	}

	confirm := messageBox(
		"Esto instalara CollaTech Agent en este equipo:\n\n"+
			"- Servicio de impresion en segundo plano\n"+
			"- Inicio automatico al encender la PC\n"+
			"- Puerto 18743 habilitado en el Firewall (red local)\n\n"+
			"¿Continuar con la instalacion?",
		appTitle, mbYesNo|mbIconQuestion)
	if confirm != idYes {
		return
	}

	result, err := install()
	if err != nil {
		messageBox("La instalacion no se completo:\n\n"+err.Error(), appTitle, mbOK|mbIconError)
		return
	}

	messageBox(successSummary(result), appTitle, mbOK|mbIconInformation)
	if messageBox("¿Abrir el panel de CollaTech Agent ahora?", appTitle, mbYesNo|mbIconQuestion) == idYes {
		shellOpen("http://localhost:18743/panel")
	}
}

func successSummary(r *installResult) string {
	var b strings.Builder
	b.WriteString("Instalacion completada.\n\n")
	fmt.Fprintf(&b, "Ubicacion: %s\n", r.installDir)
	if r.serviceOK {
		b.WriteString("Servicio de Windows: instalado y en ejecucion (auto-arranque al encender la PC)\n")
	} else {
		b.WriteString("Servicio de Windows: no se pudo registrar (revisa permisos/antivirus)\n")
	}
	if r.firewallOK {
		b.WriteString("Firewall: puerto 18743 habilitado para la red local\n")
	} else {
		b.WriteString("Firewall: no se pudo habilitar automaticamente (revisa el antivirus)\n")
	}
	b.WriteString("\nPanel en esta PC:\n  http://localhost:18743/panel\n")
	if r.hostname != "" {
		fmt.Fprintf(&b, "\nPanel desde otra PC de la red:\n  http://%s:18743/panel\n", r.hostname)
	}
	for _, ip := range r.ips {
		fmt.Fprintf(&b, "  http://%s:18743/panel\n", ip)
	}
	return b.String()
}

func install() (*installResult, error) {
	programFiles := os.Getenv("ProgramFiles")
	installDir := filepath.Join(programFiles, "CollaTech Agent")
	startMenu := filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs", "CollaTech Agent")

	agentData, err := prepareAgentBinary()
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "collatech-install")
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear carpeta temporal: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "CollaTechAgent.exe"), agentData, 0644); err != nil {
		return nil, fmt.Errorf("no se pudo escribir CollaTechAgent.exe: %w", err)
	}

	os.MkdirAll(filepath.Join(tmpDir, "configs"), 0755)
	config := `{"host":"0.0.0.0","port":18743,"log_level":"info","allowed_cors":["*"],"allow_remote":true,"max_print_size":2097152,"queue":{"workers":4,"max_retries":2},"tls":{"enabled":false,"cert_file":"certs/cert.pem","key_file":"certs/key.pem"}}`

	uninstallPs1 := fmt.Sprintf(`Add-Type -AssemblyName PresentationFramework
$nl = [Environment]::NewLine
$answer = [System.Windows.MessageBox]::Show("¿Deseas desinstalar CollaTech Agent?${nl}${nl}Esto eliminara el agente, el servicio y todos sus archivos.", "CollaTech Agent - Desinstalar", "YesNo", "Question")
if ($answer -eq "No") { exit }

# 1. Quitar accesos del menu Inicio y escritorio
Remove-Item -Path "%s" -Recurse -Force -ErrorAction SilentlyContinue
$desktop = [Environment]::GetFolderPath('Desktop')
Remove-Item -Path "$desktop\CollaTech Agent.lnk" -Force -ErrorAction SilentlyContinue

# 2. Quitar registro
Remove-Item -Path "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\CollaTechAgent" -Recurse -Force -ErrorAction SilentlyContinue

# 3. Quitar regla de firewall
netsh advfirewall firewall delete rule name="GOServer18743" | Out-Null
netsh advfirewall firewall delete rule name="CollaTech Agent 18743" | Out-Null
netsh advfirewall firewall delete rule name="CollaTech Agent App" | Out-Null

# 4. Detener y quitar el servicio de Windows
sc.exe stop CollaTechAgent | Out-Null
Start-Sleep -Milliseconds 800
sc.exe delete CollaTechAgent | Out-Null
Start-Sleep -Milliseconds 300
Stop-Process -Name "CollaTechAgent" -Force -ErrorAction SilentlyContinue

# 5. Eliminar archivos
Start-Sleep -Milliseconds 500
Remove-Item -Path "%s" -Recurse -Force -ErrorAction SilentlyContinue

# 6. Limpiar rastro del propio script
Remove-Item -Path $MyInvocation.MyCommand.Path -Force -ErrorAction SilentlyContinue

[System.Windows.MessageBox]::Show("CollaTech Agent ha sido desinstalado correctamente.", "Desinstalacion Completa", "OK", "Information")
`, startMenu, installDir)
	os.WriteFile(filepath.Join(tmpDir, "DESINSTALAR.ps1"), []byte(uninstallPs1), 0644)

	launcherBat := fmt.Sprintf(`@echo off
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Start-Process powershell.exe -ArgumentList '-NoProfile -ExecutionPolicy Bypass -File ""%s\DESINSTALAR.ps1""' -Verb RunAs -Wait"
`, installDir)
	os.WriteFile(filepath.Join(tmpDir, "Desinstalar.bat"), []byte(launcherBat), 0644)

	// Copy extra dirs from source if available (dev builds run from the repo).
	if _, err := os.Stat("go.mod"); err == nil {
		for _, dir := range []string{"profiles", "storage", "templates"} {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				copyDir(dir, filepath.Join(tmpDir, dir))
			}
		}
		if _, err := os.Stat("LOGO.png"); err == nil {
			data, _ := os.ReadFile("LOGO.png")
			os.WriteFile(filepath.Join(tmpDir, "LOGO.png"), data, 0644)
		}
	}

	os.MkdirAll(filepath.Join(tmpDir, "certs"), 0755)
	genCert(filepath.Join(tmpDir, "certs"))
	os.WriteFile(filepath.Join(tmpDir, "configs", "config.json"), []byte(config), 0644)

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("no se pudo crear %s (¿se ejecuto como administrador?): %w", installDir, err)
	}
	copyDir(tmpDir, installDir)

	os.MkdirAll(startMenu, 0755)

	createShortcut(startMenu+"\\Desinstalar.lnk", installDir+"\\Desinstalar.bat", "", installDir, 1)
	createShortcut(startMenu+"\\Panel Web.lnk", "http://localhost:18743/panel", "", "", 1)
	createShortcut(os.Getenv("USERPROFILE")+"\\Desktop\\CollaTech Agent - Panel Web.lnk", "http://localhost:18743/panel", "", "", 1)

	writeUninstallRegistry(installDir)

	firewallOK := openFirewall(filepath.Join(installDir, "CollaTechAgent.exe"))

	serviceOK := installService(filepath.Join(installDir, "CollaTechAgent.exe"))

	host, _ := os.Hostname()
	return &installResult{
		installDir: installDir,
		hostname:   host,
		ips:        localIPv4s(),
		firewallOK: firewallOK,
		serviceOK:  serviceOK,
	}, nil
}

// installService registers CollaTechAgent as a Windows Service (auto-start,
// with restart-on-crash) via sc.exe, and starts it immediately. This
// replaces a Startup-folder shortcut, which only runs after a user logs in
// and can be silently stripped by "gaming optimizer" tools or skipped by
// Windows' Fast Startup — a real service starts at boot regardless.
func installService(exePath string) bool {
	run := func(args ...string) error {
		cmd := exec.Command("sc", args...)
		cmd.SysProcAttr = hideWindow(nil)
		return cmd.Run()
	}

	_ = run("stop", serviceNameArg)
	_ = run("delete", serviceNameArg)

	if err := run("create", serviceNameArg,
		"binPath=", exePath,
		"start=", "auto",
		"DisplayName=", "CollaTech Agent",
	); err != nil {
		return false
	}
	_ = run("description", serviceNameArg, "Agente de impresion ESC/POS CollaTech (panel en http://localhost:18743/panel)")
	_ = run("failure", serviceNameArg, "reset=", "86400", "actions=", "restart/5000/restart/5000/restart/5000")

	return run("start", serviceNameArg) == nil
}

const serviceNameArg = "CollaTechAgent"

func writeUninstallRegistry(installDir string) {
	setUninstallRegistryString("DisplayName", "CollaTech Agent")
	setUninstallRegistryString("UninstallString", installDir+"\\Desinstalar.bat")
	setUninstallRegistryString("DisplayIcon", installDir+"\\CollaTechAgent.exe")
	setUninstallRegistryString("Publisher", "CollaTech")
	setUninstallRegistryString("InstallLocation", installDir)
}

// prepareAgentBinary compiles CollaTechAgent.exe from source when running
// from the repo (dev builds), otherwise falls back to the binary embedded at
// build time by BUILD_INSTALLER.bat.
func prepareAgentBinary() ([]byte, error) {
	if _, err := os.Stat("go.mod"); err == nil {
		if _, err := exec.LookPath("go"); err == nil {
			cmd := exec.Command("go", "build", "-ldflags", "-H windowsgui -s -w", "-o", "CollaTechAgent.exe", "./cmd/server/")
			cmd.SysProcAttr = hideWindow(nil)
			if cmd.Run() == nil {
				if data, err := os.ReadFile("CollaTechAgent.exe"); err == nil {
					return data, nil
				}
			}
		}
	}
	if len(agentBinary) > 0 {
		return agentBinary, nil
	}
	return nil, fmt.Errorf("no hay CollaTechAgent.exe disponible (compilalo o vuelve a generar el instalador con BUILD_INSTALLER.bat)")
}

func genCert(dir string) bool {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(5 * 365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDer, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return false
	}

	certFile, _ := os.Create(filepath.Join(dir, "cert.pem"))
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDer})
	certFile.Close()

	privBytes, _ := x509.MarshalECPrivateKey(priv)
	keyFile, _ := os.Create(filepath.Join(dir, "key.pem"))
	pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	keyFile.Close()
	return true
}

func openFirewall(programPath string) bool {
	run := func(args ...string) error {
		cmd := exec.Command("netsh", args...)
		cmd.SysProcAttr = hideWindow(nil)
		return cmd.Run()
	}

	_ = run("advfirewall", "firewall", "delete", "rule", "name=GOServer18743")
	_ = run("advfirewall", "firewall", "delete", "rule", "name=CollaTech Agent 18743")
	_ = run("advfirewall", "firewall", "delete", "rule", "name=CollaTech Agent App")

	portOK := run("advfirewall", "firewall", "add", "rule",
		"name=GOServer18743", "dir=in", "action=allow", "protocol=TCP", "localport=18743", "profile=any") == nil
	_ = run("advfirewall", "firewall", "add", "rule",
		"name=CollaTech Agent 18743", "dir=in", "action=allow", "protocol=TCP", "localport=18743", "profile=any")
	appOK := run("advfirewall", "firewall", "add", "rule",
		"name=CollaTech Agent App", "dir=in", "action=allow", "program="+programPath, "enable=yes", "profile=any") == nil

	return portOK && appOK
}

func localIPv4s() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			ip = ip.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out
}

func copyDir(src, dst string) {
	filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			os.MkdirAll(target, 0755)
		} else {
			data, _ := os.ReadFile(path)
			os.WriteFile(target, data, 0644)
		}
		return nil
	})
}

// vbsEscape doubles embedded double-quotes, VBScript's own escape convention
// for string literals. Needed because target/args/wd can themselves contain
// quoted paths (e.g. `//nologo "C:\Program Files\...\x.vbs"`), and without
// escaping those quotes terminate the VBS string early, making S.Save() a
// silent no-op (cscript fails, but its window and output are hidden).
func vbsEscape(s string) string {
	return strings.ReplaceAll(s, `"`, `""`)
}

func createShortcut(path, target, args, wd string, style int) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("CollaTechSC-%d.vbs", time.Now().UnixNano()))
	content := fmt.Sprintf(`Set WshShell = WScript.CreateObject("WScript.Shell")
Set S = WshShell.CreateShortcut("%s")
S.TargetPath = "%s"
S.Arguments = "%s"
S.WorkingDirectory = "%s"
S.WindowStyle = %d
S.Save
`, vbsEscape(path), vbsEscape(target), vbsEscape(args), vbsEscape(wd), style)
	os.WriteFile(tmpFile, []byte(content), 0644)
	cmd := exec.Command("cscript", "//nologo", tmpFile)
	cmd.SysProcAttr = hideWindow(nil)
	cmd.Run()
	os.Remove(tmpFile)
}
