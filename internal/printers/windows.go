package printers

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"
)

type WindowsPrinter struct {
	name string
}

func NewWindowsPrinter(name string) *WindowsPrinter {
	return &WindowsPrinter{name: name}
}

func (p *WindowsPrinter) Connect() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windows printer backend requires Windows")
	}
	return nil
}

func (p *WindowsPrinter) Disconnect() error { return nil }

var (
	winspool             = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinter      = winspool.NewProc("OpenPrinterA")
	procClosePrinter     = winspool.NewProc("ClosePrinter")
	procStartDocPrinter  = winspool.NewProc("StartDocPrinterA")
	procEndDocPrinter    = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter   = winspool.NewProc("EndPagePrinter")
	procWritePrinter     = winspool.NewProc("WritePrinter")
	procEnumPrinters     = winspool.NewProc("EnumPrintersA")
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

// printerInfo4A mirrors the Win32 PRINTER_INFO_4A struct: the lightweight
// enumeration level that lists printer names without querying each driver.
type printerInfo4A struct {
	pPrinterName *byte
	pServerName  *byte
	flags        uint32
}

// enumWindowsPrinters lists installed/connected printers via EnumPrinters in
// winspool.drv. This replaces shelling out to "powershell Get-Printer",
// which spawned a whole PowerShell process just to read the printer list.
func enumWindowsPrinters() []Info {
	flags := uintptr(printerEnumLocal | printerEnumConnections)

	var needed, returned uint32
	procEnumPrinters.Call(flags, 0, 4, 0, 0, uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil
	}

	buf := make([]byte, needed)
	r, _, _ := procEnumPrinters.Call(
		flags, 0, 4,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)),
	)
	if r == 0 {
		return nil
	}

	entrySize := int(unsafe.Sizeof(printerInfo4A{}))
	out := make([]Info, 0, returned)
	for i := 0; i < int(returned); i++ {
		entry := (*printerInfo4A)(unsafe.Pointer(&buf[i*entrySize]))
		name := bytePtrToString(entry.pPrinterName)
		if name == "" {
			continue
		}
		out = append(out, Info{Name: name, Type: "windows", Online: true})
	}
	return out
}

func bytePtrToString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// docInfo1A mirrors the Win32 DOC_INFO_1A struct used by StartDocPrinterA.
type docInfo1A struct {
	pDocName    *byte
	pOutputFile *byte
	pDataType   *byte
}

// Print sends raw ESC/POS bytes straight to the Windows print spooler via
// winspool.drv. This talks to the OS API directly instead of shelling out to
// PowerShell + Add-Type (which recompiled a C# helper on every single print
// job and cost 1-3+ seconds per ticket).
func (p *WindowsPrinter) Print(data []byte) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("windows printer backend requires Windows")
	}
	if len(data) == 0 {
		return nil
	}

	printerName, err := syscall.BytePtrFromString(p.name)
	if err != nil {
		return fmt.Errorf("invalid printer name %q: %w", p.name, err)
	}
	docName, _ := syscall.BytePtrFromString("CollaTech ESC/POS")
	dataType, _ := syscall.BytePtrFromString("RAW")

	var hPrinter syscall.Handle
	r, _, errno := procOpenPrinter.Call(
		uintptr(unsafe.Pointer(printerName)),
		uintptr(unsafe.Pointer(&hPrinter)),
		0,
	)
	if r == 0 {
		return fmt.Errorf("OpenPrinter failed for %q: %w", p.name, errno)
	}
	defer procClosePrinter.Call(uintptr(hPrinter))

	di := docInfo1A{pDocName: docName, pDataType: dataType}
	r, _, errno = procStartDocPrinter.Call(uintptr(hPrinter), 1, uintptr(unsafe.Pointer(&di)))
	if r == 0 {
		return fmt.Errorf("StartDocPrinter failed for %q: %w", p.name, errno)
	}
	defer procEndDocPrinter.Call(uintptr(hPrinter))

	r, _, errno = procStartPagePrinter.Call(uintptr(hPrinter))
	if r == 0 {
		return fmt.Errorf("StartPagePrinter failed for %q: %w", p.name, errno)
	}
	defer procEndPagePrinter.Call(uintptr(hPrinter))

	var written uint32
	r, _, errno = procWritePrinter.Call(
		uintptr(hPrinter),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	)
	if r == 0 {
		return fmt.Errorf("WritePrinter failed for %q: %w", p.name, errno)
	}
	if int(written) != len(data) {
		return fmt.Errorf("WritePrinter incomplete for %q: wrote %d of %d bytes", p.name, written, len(data))
	}
	return nil
}

func (p *WindowsPrinter) Status() error { return nil }
