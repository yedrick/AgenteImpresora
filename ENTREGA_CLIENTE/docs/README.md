# CollaTech Agent

[![CI](https://github.com/yedrick/AgenteImpresora/actions/workflows/ci.yml/badge.svg)](https://github.com/yedrick/AgenteImpresora/actions/workflows/ci.yml)
[![Ultima version](https://img.shields.io/github/v/release/yedrick/AgenteImpresora?label=version)](https://github.com/yedrick/AgenteImpresora/releases/latest)

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

## Descargar

Los instaladores de cada version estan en
**[Releases](https://github.com/yedrick/AgenteImpresora/releases/latest)**.
No hace falta compilar nada.

| Si tienes | Descarga | Que hacer |
|---|---|---|
| **Windows** | `CollaTechAgent-<v>-windows.zip` | descomprimir y ejecutar `INSTALADOR.exe` |
| **Ubuntu o Debian** | `collatech-agent_<v>_amd64.deb` | `sudo dpkg -i collatech-agent_*.deb` |
| **Raspberry Pi, ARM** | `collatech-agent_<v>_arm64.deb` | igual |
| **Otro Linux** | `CollaTechAgent-<v>-linux-amd64.tar.gz` | descomprimir y `sudo ./INSTALAR.sh` |
| **Mac con chip Apple** | `CollaTechAgent-<v>-macos-apple.tar.gz` | descomprimir y `sudo ./INSTALAR.sh` |
| **Mac con Intel** | `CollaTechAgent-<v>-macos-intel.tar.gz` | igual |

Con el `.deb` queda instalado, arrancado y configurado para arrancar solo con
la maquina. Al terminar, el panel esta en **http://localhost:18743/panel**.

Para comprobar que la descarga llego entera:

```bash
sha256sum -c SHA256SUMS.txt
```

## Compilar desde el codigo

Solo hace falta Go 1.22 o superior.

```bash
make build     # el agente para esta maquina
make run       # lo arranca aqui mismo
make check     # formato, vet y todas las pruebas con -race
make dist      # los seis destinos a la vez, en dist/
make install   # lo instala como servicio (pide sudo)
```

`./scripts/empaquetar.sh 1.5.0` construye los mismos paquetes que publica
GitHub Actions, para poder reproducir una version en local.

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
- **Todo `POST` debe enviar `Content-Type: application/json`** (si no, `415`).
  No es un capricho: un POST con `text/plain` es una "petición simple" para
  el navegador y no dispara comprobación previa, así que CORS no lo frena.
  Sin esto, cualquier web que abriera el cajero podía imprimir y abrir el
  cajón de dinero.
- El origen `null` nunca se autoriza: lo manda cualquier iframe con
  *sandbox*, y admitirlo dejaba leer el token desde cualquier página.
- Payload máximo configurable con `max_print_size`. Las imágenes se rechazan
  por megapíxeles **antes** de descomprimirlas.
- Límite de 30 impresiones seguidas por equipo y 10 por segundo después, para
  que un bucle mal escrito no gaste el rollo entero.
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
/api/print/layout    /api/print/raw
```

`POST /api/preview` devuelve un bloque maquetado como PNG para el diseñador.

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
[JSON-REFERENCIA.md](JSON-REFERENCIA.md) y [docs/API.md](docs/API.md). Hay
tickets de ejemplo listos para enviar en [examples/](examples/), y para
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
| `"bt://COM5"` o `"bt://rfcomm0"` | Bluetooth ya emparejada |
| `"device:///dev/usb/lp0"` | Impresora USB directa (Linux) |
| `"caja"` | Impresora dada de alta |

En Windows, las impresoras USB se direccionan por **su nombre en el sistema**,
no por el puerto: `USB001` no es un destino válido. En Linux puedes usar
directamente `/dev/usb/lp0`.

Para Bluetooth, el emparejado lo hace el sistema: en Windows la impresora
aparece como un puerto COM saliente (`bt://COM5`); en Linux se empareja con
`bluetoothctl` y se crea el nodo con `sudo rfcomm bind 0 AA:BB:CC:DD:EE:FF`
(`bt://rfcomm0`).

`GET /api/printers` lista lo que el agente detecta, con su estado real.

## Varias impresoras

Cada impresora se da de alta una vez con sus ajustes, desde el panel
(pestaña **Impresoras**) o con `POST /api/printers-config`:

```json
[
  { "name": "caja",   "target": "tcp://192.168.1.50:9100", "paper_width": 576, "cut": "partial" },
  { "name": "cocina", "target": "EPSON Cocina,EPSON Barra", "paper_width": 384, "cut": "none", "font": "b" }
]
```

Después basta con `{"printer": "caja", ...}`: el ancho, el corte y el giro
salen del perfil, así que puedes tener una de 58 mm en cocina y una de 80 mm
en caja sin repetirlo en cada llamada. Un destino con varias impresoras
separadas por coma saca una copia en cada una.

## ¿Hace falta el driver del fabricante?

**Casi nunca.** El agente habla ESC/POS directamente con la impresora:

| Conexión | Driver |
|---|---|
| Red (`tcp://host:9100`) | **No** |
| Puerto serie | **No** |
| Bluetooth | **No** (solo emparejarla en el sistema) |
| USB en Linux (`/dev/usb/lp0`) | **No** |
| USB en Windows | Hace falta una cola de impresión, y el driver **Generic / Text Only** del sistema suele bastar |

La excepción conocida es la **Star TSP100 / TSP143**: por USB en Windows sí
exige el driver futurePRNT de Star, y de fábrica no habla ESC/POS — hay que
activarle la emulación con la *TSP100 Configuration Utility*. Las versiones
LAN funcionan por `tcp://` sin nada.

### Catálogo de modelos

Lo que sí cambia de un modelo a otro, y hace que el ticket salga bien o
torcido, es el ancho de papel, si lleva cuchilla y si entiende el comando de
imagen moderno. El agente trae un catálogo con eso:

```bash
curl http://localhost:18743/api/models
```

Cubre Epson TM (T20, T82, T88, m30, U220), Star (TSP100, TSP650, mC-Print3),
Bixolon (SRP-350, SRP-330, SRP-E300), Xprinter (XP-58, XP-80), Gprinter
(GP-58, GP-80), 3nStar, Rongta y los genéricos de 58, 72 y 80 mm.

En el panel, al dar de alta una impresora eliges el modelo y se rellenan
solos el ancho y el corte. `GET /api/printers` además **reconoce el modelo**
de las impresoras detectadas por su nombre.

El catálogo avisa de los casos que dan problemas. Por ejemplo, la **Epson
TM-U220 no es térmica**: es de impacto, así que los logos y los QR no salen.
Saberlo antes ahorra una tarde.

> El agente **no descarga ni instala drivers**. Para el único caso que lo
> necesita muestra el enlace a la página oficial del fabricante y tú decides.
> Automatizarlo significaría ejecutar un instalador bajado de internet con
> permisos de administrador, y las URLs de los fabricantes cambian cada poco:
> de los seis enlaces que comprobé al montar esto, tres estaban rotos y el de
> Epson redirigía a la sección de proyectores.

## Control de la impresión

Todos los endpoints de impresión aceptan estas opciones:

| Campo | Qué hace |
|---|---|
| `width` | 384 (58 mm), 512 (72 mm) o 576 (80 mm) |
| `cut` | `"partial"`, `"full"` o `"none"` |
| `compact` | Aprieta el interlineado: ~20% menos papel por ticket |
| `upside_down` | Imprime girado 180°, para leer el ticket con la cabecera abajo |
| `feed_top` | Líneas en blanco al principio. **0 por defecto**: el agente no gasta papel arriba |
| `feed_bottom` | Líneas que se avanzan al cortar (mínimo 4, para que la cuchilla no se coma la última línea) |
| `font` | `"a"` normal, `"b"` condensada |
| `margin_dots` | Margen izquierdo en puntos |

El tamaño del QR y del código de barras **se calcula solo** según el ancho
del papel y lo que ocupe el contenido, y también se puede fijar a mano.

## Diseño libre: QR al costado, rejillas, tablas

Una impresora ESC/POS imprime **línea a línea**, así que nativamente no se
puede poner un QR al lado de un texto. Para eso está `POST /api/print/layout`:
describes el ticket como una rejilla de filas y columnas, y el agente compone
esa zona como imagen.

```json
{ "printer": "caja", "layout": { "rows": [
  { "align": "middle", "cols": [
    { "weight": 1,   "items": [ { "text": "Factura #F-000123", "bold": true } ] },
    { "dots": 150,   "items": [ { "qr": "https://kollatek.com/f/123" } ] }
  ] }
] } }
```

El **diseñador visual** en `http://localhost:18743/designer` lo arma con el
ratón, y su vista previa es exacta: la compone el mismo código que imprime.

Un bloque cuesta unos 14 KB de ráster frente a 200 bytes del mismo texto en
nativo, así que lo normal es texto nativo y solo la zona que lo necesita como
bloque. Detalle completo en [JSON-REFERENCIA.md](JSON-REFERENCIA.md).

## Cómo se imprime HTML

```text
HTML -> tokenizador propio -> comandos ESC/POS nativos -> impresora
```

El HTML se convierte directamente en comandos de texto, no en una imagen: el
ticket sale como texto real, nítido y rápido.

Se entienden encabezados, párrafos, negrita, subrayado, listas (con viñetas y
numeración), tablas con cabecera, reglas, `<pre>`, imágenes en base64 y las
etiquetas propias `<qr>`, `<barcode>` y `<feed>`. Del CSS en línea se leen
`text-align`, `font-weight`, `text-decoration` y `font-size`.

En una tabla, la última columna se alinea sola a la derecha, que en un
ticket suele ser el importe.

Ejemplos completos en [examples/html/](examples/html/).

## Soporte

Si algo falla, el panel tiene un botón **Descargar paquete de soporte** en la
pestaña Logs (o `GET /api/support-bundle`). Genera un `.zip` con el
diagnóstico del equipo, la configuración, las impresoras, la cola y los
últimos siete días de registro, con el token tapado. Es lo único que hace
falta enviar.

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
