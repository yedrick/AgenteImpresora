//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// Thin wrappers around user32/shell32/advapi32 so the installer can show
// native dialogs, self-elevate, and write registry values without spawning
// any visible console windows (cmd.exe, powershell.exe, reg.exe, etc).

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	advapi32 = syscall.NewLazyDLL("advapi32.dll")

	procMessageBoxW    = user32.NewProc("MessageBoxW")
	procIsUserAnAdmin  = shell32.NewProc("IsUserAnAdmin")
	procShellExecuteW  = shell32.NewProc("ShellExecuteW")
	procRegCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	procRegCloseKey    = advapi32.NewProc("RegCloseKey")
)

const (
	mbOK              = 0x00000000
	mbYesNo           = 0x00000004
	mbIconError       = 0x00000010
	mbIconQuestion    = 0x00000020
	mbIconInformation = 0x00000040
	mbTopMost         = 0x00040000
	mbSetForeground   = 0x00010000

	idYes = 6

	swNormal = 1

	hkeyLocalMachine     = 0x80000002
	regOptionNonVolatile = 0
	keyAllAccess         = 0xF003F
	regSZ                = 1

	// createNoWindow (CREATE_NO_WINDOW) keeps helper processes (netsh,
	// cscript, go build) from flashing a console window, since this
	// installer itself has no console to attach them to.
	createNoWindow = 0x08000000
)

func hideWindow(attr *syscall.SysProcAttr) *syscall.SysProcAttr {
	if attr == nil {
		attr = &syscall.SysProcAttr{}
	}
	attr.HideWindow = true
	attr.CreationFlags = createNoWindow
	return attr
}

// messageBox shows a native, blocking Windows dialog. Returns the pressed
// button id (idYes, IDOK=1, IDNO=7, ...).
func messageBox(text, caption string, flags uint32) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), uintptr(flags|mbTopMost|mbSetForeground))
	return int(ret)
}

// isElevated reports whether the current process token has administrator
// privileges, without spawning a helper process (unlike the old "net session" check).
func isElevated() bool {
	ret, _, _ := procIsUserAnAdmin.Call()
	return ret != 0
}

// relaunchElevated re-launches this executable with a UAC prompt. Returns
// true if Windows accepted the elevation request (the user may still cancel
// the UAC dialog itself, which the caller cannot observe here).
func relaunchElevated() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	ret, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swNormal)
	return ret > 32
}

// shellOpen opens a file/URL with its default handler (browser, explorer, etc).
func shellOpen(target string) {
	verb, _ := syscall.UTF16PtrFromString("open")
	file, _ := syscall.UTF16PtrFromString(target)
	procShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)), 0, 0, swNormal)
}

// setUninstallRegistryString writes one REG_SZ value under
// HKLM\...\Uninstall\CollaTechAgent directly via the Win32 registry API,
// replacing the old "reg add" shell-outs (five console windows per install).
func setUninstallRegistryString(name, value string) error {
	path := `Software\Microsoft\Windows\CurrentVersion\Uninstall\CollaTechAgent`
	p, _ := syscall.UTF16PtrFromString(path)
	var hKey syscall.Handle
	ret, _, _ := procRegCreateKeyEx.Call(
		uintptr(hkeyLocalMachine),
		uintptr(unsafe.Pointer(p)),
		0,
		0,
		uintptr(regOptionNonVolatile),
		uintptr(keyAllAccess),
		0,
		uintptr(unsafe.Pointer(&hKey)),
		0,
	)
	if ret != 0 {
		return fmt.Errorf("RegCreateKeyEx(%s) failed: %#x", path, ret)
	}
	defer procRegCloseKey.Call(uintptr(hKey))

	n, _ := syscall.UTF16PtrFromString(name)
	v, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	ret, _, _ = procRegSetValueEx.Call(
		uintptr(hKey),
		uintptr(unsafe.Pointer(n)),
		0,
		uintptr(regSZ),
		uintptr(unsafe.Pointer(&v[0])),
		uintptr(len(v)*2),
	)
	if ret != 0 {
		return fmt.Errorf("RegSetValueEx(%s) failed: %#x", name, ret)
	}
	return nil
}
