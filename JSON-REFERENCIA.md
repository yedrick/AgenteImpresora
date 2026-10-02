# Referencia JSON de CollaTech Agent

Cada tipo de impresión tiene **su propia URL**. No existe ningún campo `type`
en la raíz del JSON: el tipo lo determina el endpoint al que envías la
petición.

> Esta referencia estaba antes desalineada con el código (documentaba `type`,
> `templateData`, `rawData` y `rawBase64`, que el agente nunca ha aceptado).
> Lo que sigue está contrastado contra `internal/api/server.go`.

## Base

```text
http://localhost:18743
```

Desde otra PC de la red, sustituye `localhost` por el nombre o la IP del
equipo donde corre el agente, y **añade el token**:

```http
Authorization: Bearer <token>
```

El token lo muestra el instalador al terminar y el panel local en la pestaña
*Estado*. Desde la propia PC del agente no hace falta.

## Respuesta

Todos los endpoints responden con el mismo sobre:

```json
{ "ok": true, "message": "print job queued", "data": { } }
```

```json
{ "ok": false, "error": "printer and text are required" }
```

| Código | Significado |
|---|---|
| `200` | Consulta correcta |
| `202` | **Trabajo encolado** (es la respuesta normal al imprimir, no `200`) |
| `400` | JSON inválido o faltan campos |
| `401` | Falta el token o no coincide (solo desde la red) |
| `403` | Endpoint solo disponible desde la PC del agente |
| `413` | El contenido supera `max_print_size` |
| `503` | La cola está llena (revisa si la impresora responde) |

Encolar devuelve el trabajo, no el resultado de la impresión. Para saber si
salió, consulta `GET /api/status`.

## El campo `printer`

Admite el nombre de la impresora en Windows o un destino explícito:

| Valor | Destino |
|---|---|
| `"EPSON TM-T20"` | Impresora instalada en Windows |
| `"printer://EPSON TM-T20"` | Lo mismo, forzado (útil si el nombre parece un puerto) |
| `"tcp://192.168.1.50:9100"` | Red. Si omites el puerto se usa 9100 |
| `"192.168.1.50:9100"` | Red (forma corta) |
| `"COM3"` | Puerto serie |
| `"com://COM3"` | Puerto serie, forzado |
| `"cocina"` | Alias configurado en `/api/printer-aliases` |

Un alias puede apuntar a varias impresoras separadas por coma: se encola una
copia para cada una.

> Las impresoras USB se direccionan por **su nombre en Windows**, no por el
> puerto (`USB001` no es un destino válido).

## El campo `width`

Son **puntos (píxeles)**, no columnas:

| `width` | Papel | Columnas de texto |
|---|---|---|
| `384` | 58 mm | 32 |
| `512` | 72 mm | 42 |
| `576` | 80 mm | 48 |

---

## 1. Texto — `POST /api/print/text`

```json
{
  "printer": "EPSON TM-T20",
  "text": "COLLATECH\nGracias por su compra",
  "cut": true
}
```

## 2. Ticket — `POST /api/print/ticket`

Es el endpoint más completo.

```json
{
  "printer": "EPSON TM-T20",
  "title": "CAFETERIA CENTRAL",
  "width": 576,
  "cut": true,
  "drawer": false,
  "border": false,
  "feed_top": 0,
  "feed_bottom": 4,
  "margin_left": 0,
  "margin_right": 0,
  "scale": 80,
  "logo": "iVBORw0KGgo...",
  "qr": "https://kollatek.com",
  "barcode": "123456789",
  "lines": [
    { "text": "Cliente: Ana Peña", "align": "left" },
    { "text": "TOTAL", "box": true, "align": "center", "ml": 2, "mr": 2 },
    {
      "type": "table",
      "table": {
        "header": true,
        "border": true,
        "columns": [
          { "text": "Producto", "width": 20 },
          { "text": "Bs", "width": 8, "align": "right" }
        ],
        "rows": [
          ["Producto", "Bs"],
          ["Café con leche", "12.00"],
          ["Empanada", "9.50"]
        ]
      }
    }
  ]
}
```

Campos de cada elemento de `lines`:

| Campo | Tipo | Qué hace |
|---|---|---|
| `type` | `"text"` \| `"table"` | Vacío o `"text"` imprime `text`; `"table"` dibuja `table` |
| `text` | string | El texto de la línea |
| `table` | objeto | Ver arriba. Si una fila trae más celdas que columnas, sobran |
| `align` | `left` \| `center` \| `right` | Alineación |
| `bold` | bool | Negrita |
| `underline` | bool | Subrayado |
| `size` | `normal` \| `double` \| `wide` \| `tall` \| `small` | Tamaño |
| `gap` | int | Líneas en blanco después |
| `box` | bool | Enmarca el texto |
| `ml` / `mr` | int | Margen izquierdo / derecho, en caracteres |

Límites: `qr` hasta 2953 caracteres, `barcode` hasta 253. Pasarse devuelve
`400`. `feed_bottom` tiene un mínimo de 4 líneas para que la cuchilla no corte
la última línea.

## 3. Plantilla — `POST /api/print/template`

El campo con los valores se llama **`data`**.

```json
{
  "printer": "EPSON TM-T20",
  "template": "factura",
  "width": 576,
  "cut": true,
  "data": {
    "empresa": "CollaTech",
    "cliente": "Ana Peña",
    "total": "Bs 21.50",
    "mensaje": "Gracias por su compra",
    "qr": "https://kollatek.com",
    "barcode": "123456789",
    "items": [
      { "nombre": "Café con leche", "precio": "Bs 12.00" },
      { "nombre": "Empanada", "precio": "Bs 9.50" }
    ]
  }
}
```

Plantillas disponibles — las devuelve `GET /api/templates`:

`factura`, `recibo`, `comanda`, `texto`, `qr`, `imagen`.

Son diseños **integrados en el agente**, no archivos que puedas editar.
Cualquier nombre no reconocido imprime `factura`. Para un diseño propio, usa
`/api/print/ticket` o `/api/print/html`.

## 4. HTML — `POST /api/print/html`

```json
{
  "printer": "EPSON TM-T20",
  "html": "<!DOCTYPE html><html><body><h1>CollaTech</h1><p>Total: <b>Bs 120</b></p></body></html>",
  "width": 576,
  "cut": true
}
```

Soporta `h1`-`h6`, `p`, `div`, `center`, `b`/`strong`, `i`/`em`/`u`, `br`,
`hr`, `li` y `table`/`tr`/`td`/`th`. Del CSS solo lee `text-align`. El
contenido de `<style>`, `<script>`, `<title>` y `<head>` se ignora. Los saltos
de línea del fuente no son saltos de línea del ticket: usa `<br>` o bloques.

## 5. Imagen — `POST /api/print/image`

```json
{
  "printer": "EPSON TM-T20",
  "image": "data:image/png;base64,iVBORw0KGgo...",
  "width": 576,
  "scale": 80,
  "cut": true
}
```

PNG, JPEG o GIF, en base64 o como data URI. `scale` va de 35 a 100.

## 6. Logo — `POST /api/print/logo`

Imprime el logo. Si pones un `LOGO.png` junto al ejecutable se usa ese;
si no, se imprime el que trae el agente incorporado.

```json
{ "printer": "EPSON TM-T20", "width": 576, "scale": 80, "cut": true }
```

## 7. ESC/POS crudo — `POST /api/print/raw`

Los campos son **`data`** y **`base64`**.

```json
{ "printer": "EPSON TM-T20", "data": "G0BDT0xMQVRFQ0gKHVYA", "base64": true }
```

Con `"base64": false`, `data` se trata como texto y se codifica en CP850 (los
bytes ASCII y de control pasan tal cual).

---

## Consultas

| Endpoint | Devuelve |
|---|---|
| `GET /health` | `{"ok":true,"message":"healthy"}` (sin `data`) |
| `GET /api/status` | Servicio, hora y los trabajos en cola |
| `GET /api/printers` | Impresoras detectadas, con `online` y `status` |
| `GET /api/templates` | Nombres de las plantillas integradas |
| `GET /api/network` | Hostname, IPs y URLs del panel |
| `GET /api/settings` | Ancho de papel, escala y alias |
| `GET /api/printer-aliases` | Solo los alias |

### Solo desde la PC del agente

Estos responden `403` desde la red, aunque mandes el token:

| Endpoint | Para qué |
|---|---|
| `GET /api/diagnostico` | Informe completo para soporte |
| `GET /api/logs?limit=80` | Últimas líneas del log (máximo 500) |
| `GET /api/token` | Consultar el token de acceso |
| `POST /api/settings` | Cambiar ancho de papel y escala |
| `POST /api/printer-aliases` | Cambiar los alias |

## Alias de impresora

```json
[
  { "name": "cocina", "printer": "EPSON Cocina,EPSON Barra", "description": "Pedidos a cocina y barra" },
  { "name": "facturas", "printer": "EPSON Caja", "description": "Facturacion" }
]
```

Enviándolo a `POST /api/printer-aliases`, tu sistema puede luego imprimir con
`{"printer": "cocina", ...}` y sale una copia en cada impresora del alias.
