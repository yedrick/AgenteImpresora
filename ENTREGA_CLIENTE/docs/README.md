# CollaTech Agent

CollaTech Agent es un agente local de impresión ESC/POS. Expone una API REST
y convierte trabajos POS a bytes ESC/POS para impresoras térmicas.

Funciona en **Windows, Linux y macOS**: usa el spooler del sistema (winspool
en Windows, CUPS en Linux y macOS) y también sabe imprimir por TCP/IP, por
puerto serie y directamente sobre un dispositivo USB.

Por defecto escucha solo en `127.0.0.1:18743`. El instalador lo configura para
la red local (`0.0.0.0`) y, en ese caso, **genera un token de acceso** que las
peticiones desde otras PCs deben enviar.

## Requisitos

- Una impresora térmica compatible ESC/POS
- En Linux y macOS, CUPS instalado si vas a imprimir por el spooler
  (`lp` y `lpstat`); para TCP/IP o USB directo no hace falta
- Go 1.22 o superior solo para compilar

## Instalación

### Linux y macOS

Un solo comando: el binario se instala a sí mismo como servicio.

```bash
sudo ./collatech-agent --install
```

Eso copia el binario a `/usr/local/bin`, crea la configuración en
`/etc/collatech-agent/config.json` (en macOS, `/usr/local/etc/...`) con un
token generado que te muestra por pantalla, deja los datos en
`/var/lib/collatech-agent`, y registra el servicio: unidad **systemd** en
Linux o demonio **launchd** en macOS, con arranque automático y reinicio
ante fallo.

```bash
sudo ./collatech-agent --uninstall   # quitarlo
systemctl status collatech-agent     # ver cómo va (Linux)
journalctl -u collatech-agent -f     # seguir el registro (Linux)
```

Para imprimir por el spooler, la cola de CUPS debe estar configurada como
**raw** (sin driver), que es lo normal en una térmica ESC/POS:

```bash
lpadmin -p POS1 -E -v socket://192.168.1.50:9100 -m raw
```

### Windows

Entrega un único archivo:

```text
INSTALADOR.exe
```

El cliente lo ejecuta con clic derecho → **Ejecutar como administrador**.
Si no se ejecuta elevado, el instalador lo indica y se detiene.

El instalador:

- instala `CollaTechAgent.exe` en `%ProgramFiles%\CollaTech Agent`
- lo registra como servicio de Windows con auto-arranque y reinicio ante fallo
- configura acceso LAN en `0.0.0.0:18743`
- **genera un token de acceso y lo muestra al terminar**
- abre el puerto `18743/TCP` en el Firewall, en los perfiles Privado y Dominio
- crea accesos directos y entrada de desinstalación
- muestra la ruta local y la ruta de red por nombre de PC/IP

## Panel

```text
http://localhost:18743/panel        Panel de pruebas y configuración
http://localhost:18743/designer     Diseñador visual de tickets
http://localhost:18743/diagnostico  Informe para soporte
```

Desde otra PC de la red, sustituye `localhost` por el nombre o la IP del
equipo. El panel pedirá el token la primera vez.

## Seguridad

- **Modo local** (por defecto): el agente se enlaza a `127.0.0.1` y rechaza
  cualquier conexión que no venga de la propia PC.
- **Modo LAN** (`host: "0.0.0.0"`, `allow_remote: true`): se exige un token en
  todo `/api/*` a las peticiones que llegan desde la red. Si no hay token
  configurado, el agente genera uno al arrancar y lo guarda en
  `configs/config.json`. Lo consultas en el panel local, pestaña *Estado*, o
  en `GET /api/token`.
- **Siempre solo desde la propia PC**, aunque `allow_remote` esté activo:
  `/api/diagnostico`, `/api/logs`, `/api/token` y los `POST` de configuración.
- CORS con lista de orígenes exactos en `configs/config.json`. Evita `"*"`:
  con comodín, cualquier web que abra el cajero puede imprimir y abrir el
  cajón de dinero.
- Payload máximo configurable con `max_print_size`.
- TLS opcional. Para usarlo en red, genera el certificado con
  `GENERAR_CERTIFICADO.bat`, que incluye SAN.

## Configuración — `configs/config.json`

```json
{
  "host": "0.0.0.0",
  "port": 18743,
  "allow_remote": true,
  "auth_token": "",
  "allowed_cors": ["http://localhost:18743"],
  "max_print_size": 2097152,
  "queue": { "workers": 4, "max_retries": 2 },
  "tls": { "enabled": false, "cert_file": "certs/cert.pem", "key_file": "certs/key.pem" }
}
```

Los trabajos hacia **una misma impresora** siempre salen en orden, uno detrás
de otro, sea cual sea el número de `workers`; los `workers` solo permiten
imprimir en paralelo en impresoras distintas.

`max_retries` es el número **total de intentos**, no de reintentos: con `2`,
el agente prueba una vez, espera 1 segundo y prueba una segunda vez.

## Opciones de línea de comandos

```text
--config RUTA     archivo de configuración (por defecto, junto al ejecutable)
--data-dir RUTA   carpeta para logs/ y storage/
--host HOST       dirección de escucha, por encima del archivo
--port N          puerto de escucha, por encima del archivo
--token T         token de acceso, por encima del archivo
--install         instalar como servicio del sistema y arrancarlo
--uninstall       detener y quitar el servicio
--version         mostrar la versión
```

## Desarrollo

```bash
go mod tidy
go run ./cmd/server
go test ./...
go test -race ./...
go test -run=NONE -bench=. ./internal/escpos ./internal/render ./internal/api
```

Compila y pasa los tests en Linux, macOS y Windows.

### Compilación

```bash
# Linux / macOS
go build -ldflags "-s -w" -o collatech-agent ./cmd/server

# Windows, desde cualquier sistema
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o CollaTechAgent.exe ./cmd/server
```

La versión se incrusta con `-ldflags "-X main.version=1.2.0"`.

Para regenerar el instalador hay que compilar **primero** el agente: usa
`BUILD_SERVER.bat` y después `BUILD_INSTALLER.bat`, que embebe el binario.

## Estructura

```text
collatech-agent/
  cmd/server         agente, flags e instalación del servicio por sistema
  cmd/installer      instalador gráfico de Windows, con el agente embebido
  internal/api       API REST y panel web embebido
    server.go          montaje, rutas y estado compartido
    middleware.go      acceso, token y CORS
    handlers_*.go      manejadores por área
    ticket.go          composición de tickets y plantillas
    diagnostics.go     informe de soporte y lectura del registro
  internal/escpos    encoder ESC/POS (CP850, QR, barcode, raster)
  internal/render    conversor HTML -> ESC/POS
  internal/printers  spooler (winspool / CUPS), TCP y dispositivos
  internal/queue     cola con reintentos y persistencia
  internal/config    configuración y rutas por sistema
  collatech-sdk      SDK TypeScript
  configs storage logs docs
```

## Endpoints

Impresión (todos `POST`, responden **202 Accepted**):

```text
/api/print/text      /api/print/ticket    /api/print/template
/api/print/html      /api/print/image     /api/print/logo
/api/print/raw
```

Consulta (`GET`):

```text
/health  /api/status  /api/printers  /api/templates  /api/network
/api/settings  /api/printer-aliases
```

Configuración (`POST`) y administración (`GET`), **solo desde la propia PC**:

```text
POST /api/settings   POST /api/printer-aliases
GET  /api/diagnostico  GET /api/logs  GET /api/token
```

El contrato completo, campo a campo, está en
[JSON-REFERENCIA.md](JSON-REFERENCIA.md) y [docs/API.md](docs/API.md). Para
integrar desde Angular, [ANGULAR.md](ANGULAR.md).

## Ejemplos curl

Texto:

```bash
curl -X POST http://localhost:18743/api/print/text \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","text":"COLLATECH\nGracias por su compra","cut":true}'
```

Ticket con QR y código de barras:

```bash
curl -X POST http://localhost:18743/api/print/ticket \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","title":"COLLATECH","qr":"https://kollatek.com","barcode":"123456789","cut":true}'
```

HTML:

```bash
curl -X POST http://localhost:18743/api/print/html \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","html":"<h1>CollaTech</h1><p>Total: <b>Bs 120</b></p>","width":576,"cut":true}'
```

Desde otra PC de la red, añade el token:

```bash
curl -X POST http://192.168.1.50:18743/api/print/text \
  -H "Authorization: Bearer TU_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","text":"Hola","cut":true}'
```

Alias de impresora por estación:

```bash
curl -X POST http://localhost:18743/api/printer-aliases \
  -H "Content-Type: application/json" \
  -d '[{"name":"cocina","printer":"EPSON Cocina,EPSON Barra","description":"Pedidos a cocina y barra"}]'
```

Después basta con imprimir usando el alias; si apunta a varias impresoras, se
encola una copia para cada una:

```bash
curl -X POST http://localhost:18743/api/print/text \
  -H "Content-Type: application/json" \
  -d '{"printer":"cocina","text":"Pedido #1001\nMesa 4","cut":true}'
```

## Destinos de impresión

Valores aceptados en `printer`:

| Valor | Destino |
|---|---|
| `"EPSON TM-T20"` | Cola del spooler del sistema (winspool o CUPS) |
| `"printer://EPSON TM-T20"` | Lo mismo, forzado |
| `"tcp://192.168.1.50:9100"` o `"192.168.1.50:9100"` | Red (puerto 9100 por defecto) |
| `"COM3"` o `"com://COM3"` | Puerto serie (Windows) |
| `"/dev/ttyUSB0"` | Puerto serie (Linux / macOS) |
| `"device:///dev/usb/lp0"` | Impresora USB directa (Linux) |
| `"cocina"` | Alias de estación |

En Windows, las impresoras USB se direccionan por **su nombre en el sistema**,
no por el puerto: `USB001` no es un destino válido. En Linux puedes usar
directamente `/dev/usb/lp0`.

`GET /api/printers` lista lo que el agente detecta, con su estado real.

## Cómo se imprime HTML

```text
HTML -> tokenizador propio -> comandos ESC/POS nativos -> impresora
```

El HTML se convierte directamente en comandos de texto ESC/POS, no en una
imagen: el ticket sale como texto real, nítido y rápido. Eso también marca el
límite: se soportan los bloques y el formato básico (encabezados, párrafos,
negrita, subrayado, listas, reglas y tablas) y, del CSS, solo `text-align`.
Para un diseño con control fino, usa `/api/print/ticket` o
`/api/print/image`.

## Logs

Se guardan en `logs/YYYY-MM-DD.jsonl`, con rotación diaria y 30 días de
retención:

```json
{"time":"2026-10-02T12:07:29.35-04:00","level":"info","event":"print_ticket","details":{"printer":"POS1","lines":3}}
```

Se registra la **forma** de cada trabajo (impresora, número de líneas,
tamaños, estados, reintentos y errores), nunca el contenido impreso: los
nombres de clientes y los importes no quedan en el log.

## Cola

Los trabajos pasan por `Pending` → `Printing` → `Completed` / `Failed`, con
reintentos automáticos y espera creciente entre intentos. Se conservan los
500 más recientes. Si la cola se llena —normalmente porque una impresora dejó
de responder—, la API devuelve `503` en vez de quedarse esperando.
