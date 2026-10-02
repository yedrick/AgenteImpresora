//go:build windows

package printers

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// En Windows el spooler del sistema es winspool.drv.
type winspoolPrinter struct {
	name string
}

func newSpoolerPrinter(name string) Printer { return &winspoolPrinter{name: name} }

func (p *winspoolPrinter) Connect() error    { return nil }
func (p *winspoolPrinter) Disconnect() error { return nil }

// Se usa NewLazySystemDLL (no NewLazyDLL) para que winspool.drv se resuelva
// siempre desde System32: el agente corre como servicio con privilegios altos
// y NewLazyDLL busca primero junto al ejecutable, lo que permite secuestrar la
// DLL dejando un archivo con ese nombre al lado del .exe.
var (
	winspool             = windows.NewLazySystemDLL("winspool.drv")
	procOpenPrinter      = winspool.NewProc("OpenPrinterW")
	procClosePrinter     = winspool.NewProc("ClosePrinter")
	procGetPrinter       = winspool.NewProc("GetPrinterW")
	procStartDocPrinter  = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter    = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter   = winspool.NewProc("EndPagePrinter")
	procWritePrinter     = winspool.NewProc("WritePrinter")
	procEnumPrinters     = winspool.NewProc("EnumPrintersW")
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004

	statusPaused            = 0x00000001
	statusError             = 0x00000002
	statusPaperJam          = 0x00000008
	statusPaperOut          = 0x00000010
	statusPaperProblem      = 0x00000040
	statusOffline           = 0x00000080
	statusOutOfMemory       = 0x00000200
	statusNotAvailable      = 0x00001000
	statusUserIntervention  = 0x00100000
	statusDoorOpen          = 0x00400000
	statusNoToner           = 0x00040000
	statusOutOfPaperOrError = statusError | statusPaperJam | statusPaperOut |
		statusPaperProblem | statusOffline | statusNotAvailable |
		statusDoorOpen | statusUserIntervention | statusOutOfMemory
)

// printerInfo4W refleja PRINTER_INFO_4W: el nivel ligero que lista nombres sin
// consultar cada driver. Se usa la variante W (UTF-16) porque la ANSI recibia
// los nombres en UTF-8 y las impresoras con acentos ("Impresion Caja",
// "Deposito") no se encontraban ni se listaban bien.
type printerInfo4W struct {
	pPrinterName *uint16
	pServerName  *uint16
	flags        uint32
}

// docInfo1W refleja DOC_INFO_1W, usado por StartDocPrinterW.
type docInfo1W struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDataType   *uint16
}

func enumSpoolerPrinters() []Info {
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

	entrySize := int(unsafe.Sizeof(printerInfo4W{}))
	out := make([]Info, 0, returned)
	for i := 0; i < int(returned); i++ {
		if (i+1)*entrySize > len(buf) {
			break
		}
		entry := (*printerInfo4W)(unsafe.Pointer(&buf[i*entrySize]))
		name := windows.UTF16PtrToString(entry.pPrinterName)
		if name == "" {
			continue
		}
		online, detail := printerStatus(name)
		out = append(out, Info{Name: name, Type: "winspool", Online: online, Status: detail})
	}
	// Los punteros de la lista apuntan dentro de buf; hay que mantenerlo vivo
	// hasta terminar de leerlos.
	runtime.KeepAlive(buf)
	return out
}

// printerStatus consulta GetPrinterW nivel 6 (solo el DWORD de estado). Antes
// se devolvia Online: true a secas, asi que el panel daba por buena una
// impresora apagada o sin papel.
func printerStatus(name string) (bool, string) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, "nombre no valido"
	}
	var h windows.Handle
	r, _, _ := procOpenPrinter.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&h)), 0)
	if r == 0 {
		return false, "no se pudo abrir"
	}
	defer procClosePrinter.Call(uintptr(h))

	var status uint32
	var needed uint32
	r, _, _ = procGetPrinter.Call(uintptr(h), 6,
		uintptr(unsafe.Pointer(&status)), unsafe.Sizeof(status),
		uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		// Sin estado disponible: se informa como disponible pero sin detalle,
		// que es lo unico honesto que se puede decir.
		return true, ""
	}
	return status&statusOutOfPaperOrError == 0, describeStatus(status)
}

func describeStatus(status uint32) string {
	switch {
	case status == 0:
		return "lista"
	case status&statusOffline != 0:
		return "sin conexion"
	case status&statusPaperOut != 0:
		return "sin papel"
	case status&statusPaperJam != 0:
		return "papel atascado"
	case status&statusDoorOpen != 0:
		return "tapa abierta"
	case status&statusPaperProblem != 0:
		return "problema de papel"
	case status&statusNoToner != 0:
		return "sin tinta"
	case status&statusUserIntervention != 0:
		return "requiere atencion"
	case status&statusNotAvailable != 0:
		return "no disponible"
	case status&statusOutOfMemory != 0:
		return "sin memoria"
	case status&statusError != 0:
		return "error"
	case status&statusPaused != 0:
		return "en pausa"
	default:
		return "ocupada"
	}
}

// Print manda los bytes ESC/POS al spooler de Windows como trabajo RAW.
func (p *winspoolPrinter) Print(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("no hay datos que enviar a %q", p.name)
	}

	printerName, err := windows.UTF16PtrFromString(p.name)
	if err != nil {
		return fmt.Errorf("nombre de impresora no valido %q: %w", p.name, err)
	}
	docName, _ := windows.UTF16PtrFromString("CollaTech ESC/POS")
	dataType, _ := windows.UTF16PtrFromString("RAW")

	var hPrinter windows.Handle
	r, _, errno := procOpenPrinter.Call(
		uintptr(unsafe.Pointer(printerName)),
		uintptr(unsafe.Pointer(&hPrinter)),
		0,
	)
	if r == 0 {
		return fmt.Errorf("OpenPrinter fallo para %q: %w", p.name, errno)
	}
	defer procClosePrinter.Call(uintptr(hPrinter))

	di := docInfo1W{pDocName: docName, pDataType: dataType}
	r, _, errno = procStartDocPrinter.Call(uintptr(hPrinter), 1, uintptr(unsafe.Pointer(&di)))
	if r == 0 {
		return fmt.Errorf("StartDocPrinter fallo para %q: %w", p.name, errno)
	}
	defer procEndDocPrinter.Call(uintptr(hPrinter))

	r, _, errno = procStartPagePrinter.Call(uintptr(hPrinter))
	if r == 0 {
		return fmt.Errorf("StartPagePrinter fallo para %q: %w", p.name, errno)
	}
	defer procEndPagePrinter.Call(uintptr(hPrinter))

	// El spooler puede aceptar menos bytes de los pedidos. Antes eso se
	// trataba como error y la cola reintentaba el trabajo entero, imprimiendo
	// el ticket dos veces.
	for sent := 0; sent < len(data); {
		var written uint32
		r, _, errno = procWritePrinter.Call(
			uintptr(hPrinter),
			uintptr(unsafe.Pointer(&data[sent])),
			uintptr(len(data)-sent),
			uintptr(unsafe.Pointer(&written)),
		)
		if r == 0 {
			return fmt.Errorf("WritePrinter fallo para %q tras %d de %d bytes: %w", p.name, sent, len(data), errno)
		}
		if written == 0 {
			return fmt.Errorf("WritePrinter no avanzo para %q tras %d de %d bytes", p.name, sent, len(data))
		}
		sent += int(written)
	}
	return nil
}

func (p *winspoolPrinter) Status() error {
	if online, detail := printerStatus(p.name); !online {
		return fmt.Errorf("impresora %q no disponible: %s", p.name, detail)
	}
	return nil
}

// detectDevices busca puertos serie. Abrirlos es la unica forma de saber si
// existen en Windows, asi que el resultado se cachea en Manager.List para no
// molestar a una impresora serie que este imprimiendo.
func detectDevices() []Info {
	var out []Info
	for i := 1; i <= 32; i++ {
		name := fmt.Sprintf("COM%d", i)
		path := devicePath(name)
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		_ = f.Close()
		out = append(out, Info{Name: name, Type: "serial", Address: path, Online: true, Status: "disponible"})
	}
	return out
}
