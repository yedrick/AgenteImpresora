# API de CollaTech Agent

Base: `http://localhost:18743`

Desde otra PC de la red, usa el nombre o la IP del equipo donde corre el
agente y añade la cabecera del token:

```http
Authorization: Bearer <token>
```

El token lo muestra el instalador al terminar y el panel local en la pestaña
*Estado* (`GET /api/token`). Desde la propia PC del agente no hace falta.

## Formato de respuesta

```json
{ "ok": true, "message": "print job queued", "data": {} }
```

```json
{ "ok": false, "error": "printer and text are required" }
```

| Código | Cuándo |
|---|---|
| `200` | Consulta correcta |
| `202` | **Trabajo encolado.** Es la respuesta normal de `/api/print/*` |
| `400` | JSON inválido, faltan campos, o QR/código de barras demasiado largo |
| `401` | Falta el token o no coincide (solo aplica desde la red) |
| `403` | Endpoint de administración invocado desde la red |
| `413` | El contenido supera `max_print_size` |
| `429` | Demasiadas impresiones seguidas desde ese equipo |
| `503` | La cola está llena |

Encolar no significa haber impreso. Para el resultado real, consulta
`GET /api/status` o los logs.

## Endpoints

### Impresión — `POST`, responden `202`

| Ruta | Campos |
|---|---|
| `/api/print/text` | `printer`, `text`, `cut` |
| `/api/print/ticket` | `printer`, `title`, `lines[]`, `qr`, `barcode`, `logo`, `scale`, `border`, `margin_left`, `margin_right` + opciones de documento |
| `/api/print/template` | `printer`, `template`, `data{}`, `width`, `cut` |
| `/api/print/html` | `printer`, `html`, `width`, `cut` |
| `/api/print/image` | `printer`, `image`, `width`, `scale`, `cut` |
| `/api/print/logo` | `printer`, `width`, `scale`, `cut` |
| `/api/print/raw` | `printer`, `data`, `base64` |

Todos los endpoints de impresión aceptan además las **opciones de
documento**: `width`, `cut` (`partial`/`full`/`none` o `true`/`false`),
`compact`, `line_spacing`, `upside_down`, `feed_top`, `feed_bottom`,
`margin_dots`, `font` y `drawer`.

### Consulta — `GET`

| Ruta | Devuelve |
|---|---|
| `/health` | `{"ok":true,"message":"healthy"}`. **No trae `data`** |
| `/api/status` | `service`, `time` y `jobs[]` de la cola |
| `/api/printers` | `name`, `type`, `address`, `online`, `status`, y `model`/`model_name`/`paper_width` si se reconoce el modelo |
| `/api/templates` | Nombres de las plantillas integradas |
| `/api/models` | Catálogo de modelos conocidos: ancho, corte, si admite imágenes y si necesita driver |
| `/api/network` | `hostname`, `host`, `port`, `allow_remote`, `urls[]` |
| `/api/settings` | `default_printer`, `paper_width`, `image_scale`, `aliases[]` |
| `/api/printer-aliases` | Solo `aliases[]` |

### Administración — solo desde la PC del agente

Responden `403` desde la red, incluso con el token correcto.

| Ruta | Para qué |
|---|---|
| `GET /api/diagnostico` | Informe completo para soporte (el token va tapado) |
| `GET /api/support-bundle` | Un .zip con diagnóstico, configuración, impresoras, cola y registro |
| `GET /api/logs?limit=80` | Últimas líneas del log. Máximo 500 |
| `GET /api/token` | Consultar el token de acceso |
| `POST /api/settings` | Cambiar ancho de papel y escala |
| `GET`+`POST /api/printers-config` | Impresoras dadas de alta, con su papel, corte y giro |
| `POST /api/printer-aliases` | Compatibilidad: solo nombre y destino |

### Páginas

`GET /`, `/panel`, `/designer`, `/diagnostico`.

## Unidades

`width` son **puntos**, no columnas: `384` = 58 mm (32 columnas),
`512` = 72 mm (42 columnas), `576` = 80 mm (48 columnas). `scale` es un
porcentaje de 35 a 100.

Límites: `qr` hasta 2953 caracteres, `barcode` hasta 253. Pasarse devuelve
`400`.

## Destinos de impresión

| Valor de `printer` | Destino |
|---|---|
| `"EPSON TM-T20"` | Impresora instalada en Windows |
| `"printer://EPSON TM-T20"` | Lo mismo, forzado |
| `"tcp://192.168.1.50:9100"` | Red. Sin puerto se usa 9100 |
| `"COM3"` / `"com://COM3"` | Puerto serie en Windows |
| `"/dev/ttyUSB0"` | Puerto serie en Linux / macOS |
| `"bt://COM5"` / `"bt://rfcomm0"` | Bluetooth ya emparejada |
| `"device:///dev/usb/lp0"` | USB directa en Linux |
| `"cocina"` | Alias de estación |

Las impresoras USB se direccionan por su nombre en Windows; `USB001` es un
puerto del spooler, no un destino válido.

## Alias de impresora

Permiten manejar estaciones lógicas: `cocina`, `recepcion`, `facturas`,
`pagos`.

```json
[
  { "name": "cocina", "printer": "EPSON Cocina,EPSON Barra", "description": "Pedidos" },
  { "name": "facturas", "printer": "EPSON Caja", "description": "Facturacion" }
]
```

Al imprimir se puede enviar:

```json
{ "printer": "cocina", "text": "Pedido #1001", "cut": true }
```

El agente resuelve `cocina` a una o varias impresoras reales antes de encolar.
Si hay varias separadas por coma, crea un trabajo para cada una. La cola
trabaja en paralelo entre impresoras distintas y mantiene el orden dentro de
cada impresora.

## Plantillas

`GET /api/templates` devuelve `factura`, `recibo`, `comanda`, `texto`, `qr`,
`imagen`. Son diseños integrados en el agente, no archivos editables; un
nombre no reconocido imprime `factura`.

El contrato completo, con ejemplos de cada endpoint, está en
[JSON-REFERENCIA.md](../JSON-REFERENCIA.md).
