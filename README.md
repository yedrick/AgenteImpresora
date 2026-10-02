# CollaTech Agent

CollaTech Agent es un agente local de impresion ESC/POS para Windows x64. Expone una API REST solo en `localhost:18743` y convierte trabajos POS a bytes ESC/POS para impresoras termicas Windows, TCP/IP, COM y adaptadores USB tipo archivo.

## Requisitos

- Go 1.22 o superior
- Windows x64 para uso en produccion con impresoras locales
- Impresora termica compatible ESC/POS

## Instalacion

### Cliente final

Entrega solo este archivo:

```text
INSTALADOR.exe
```

El cliente debe ejecutarlo con clic derecho:

```text
Ejecutar como administrador
```

El instalador hace todo:

- instala `CollaTechAgent.exe`
- configura acceso LAN en `0.0.0.0:18743`
- abre el puerto `18743/TCP` en Firewall de Windows
- crea auto-arranque al iniciar Windows
- crea accesos directos
- muestra la ruta local y la ruta de red por nombre de PC/IP

Si no se ejecuta como administrador, el instalador muestra el requisito y se detiene.

```powershell
go mod tidy
go run ./cmd/server
```

El servidor escucha en:

```text
http://localhost:18743
```

Panel HTML de pruebas:

```text
http://localhost:18743/panel
```

Acceso desde otra PC de la red:

```text
http://IP-DE-ESTA-PC:18743/panel
```

Ejemplo:

```text
http://192.168.1.50:18743/panel
```

La configuracion LAN esta en `configs/config.json`:

```json
{
  "host": "0.0.0.0",
  "allow_remote": true,
  "queue": { "workers": 1 }
}
```

Con `workers: 1`, las impresiones salen en cola: una termina y luego empieza la siguiente.

Disenador visual:

```text
http://localhost:18743/designer
```

## Compilacion Windows x64

```powershell
$env:GOOS="windows"
$env:GOARCH="amd64"
go build -o CollaTechAgent.exe ./cmd/server
```

## Estructura

```text
collatech-agent/
  cmd/server/main.go
  internal/api
  internal/escpos
  internal/printers
  internal/render
  internal/queue
  internal/profiles
  internal/logs
  internal/updater
  templates
  configs
  storage
  docs
```

## Endpoints

- `GET /api/printers`
- `GET /api/printer-aliases`
- `POST /api/printer-aliases`
- `POST /api/print/text`
- `POST /api/print/ticket`
- `POST /api/print/template`
- `POST /api/print/html`
- `POST /api/print/image`
- `POST /api/print/raw`
- `GET /api/status`
- `GET /health`

## Ejemplos curl

Texto:

```bash
curl -X POST http://localhost:18743/api/print/text \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","text":"COLLATECH\nGracias por su compra","cut":true}'
```

Ticket con QR y barcode:

```bash
curl -X POST http://localhost:18743/api/print/ticket \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","title":"COLLATECH","qr":"https://kollatek.com","barcode":"123456789","cut":true}'
```

HTML:

```bash
curl -X POST http://localhost:18743/api/print/html \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","html":"<html><body><h1>CollaTech</h1><p>Total: Bs 120</p></body></html>","width":576,"cut":true}'
```

Raw ESC/POS en base64:

```bash
curl -X POST http://localhost:18743/api/print/raw \
  -H "Content-Type: application/json" \
  -d '{"printer":"EPSON","data":"G0BDT0xMQVRFQ0gKHVYA","base64":true}'
```

Alias de impresora por estacion:

```bash
curl -X POST http://localhost:18743/api/printer-aliases \
  -H "Content-Type: application/json" \
  -d '[{"name":"cocina","printer":"EPSON Cocina,EPSON Barra","description":"Pedidos a cocina y barra"},{"name":"facturas","printer":"EPSON Caja","description":"Facturacion"}]'
```

Luego tu sistema puede imprimir usando el alias. Si el alias tiene varias impresoras separadas por coma, se encola una copia para cada impresora:

```bash
curl -X POST http://localhost:18743/api/print/text \
  -H "Content-Type: application/json" \
  -d '{"printer":"cocina","text":"Pedido #1001\nMesa 4","cut":true}'
```

## Flujo HTML a impresion

```text
HTML -> Render -> Imagen -> Raster ESC/POS -> Impresora
```

El modulo `internal/render` incluye un renderizador interno liviano para convertir HTML simple a imagen raster ESC/POS sin dependencias externas. En produccion se puede sustituir por Chromium/WebView2 headless manteniendo la misma interfaz.

## Impresoras

Formatos aceptados en `printer`:

- Nombre de impresora Windows: `"EPSON TM-T20"`
- TCP/IP: `"tcp://192.168.1.50:9100"` o `"192.168.1.50:9100"`
- COM: `"COM3"`
- USB/adaptador archivo: `"USB001"`

## Seguridad

- En modo local, el servidor se enlaza a `127.0.0.1` y rechaza conexiones no locales
- En modo LAN, `configs/config.json` usa `host: "0.0.0.0"` y `allow_remote: true`
- CORS configurable en `configs/config.json`
- Payload maximo configurable con `max_print_size`
- JSON validado por endpoint

## Logs

Los eventos se guardan en `logs/YYYY-MM-DD.jsonl` en formato JSON Lines:

- impresiones encoladas
- cambios de estado
- errores
- reintentos
- impresora utilizada

## Cola

Los trabajos pasan por:

- `Pending`
- `Printing`
- `Completed`
- `Failed`

La cola usa reintentos automaticos con backoff exponencial.
