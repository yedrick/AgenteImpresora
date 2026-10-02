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
| `429` | Demasiadas impresiones seguidas desde ese equipo (30 de golpe, luego 10/s) |
| `503` | La cola está llena (revisa si la impresora responde) |

Encolar devuelve el trabajo, no el resultado de la impresión. Para saber si
salió, consulta `GET /api/status`.

## El campo `printer`

Admite el nombre de la impresora en Windows o un destino explícito:

| Valor | Destino |
|---|---|
| `"EPSON TM-T20"` | Cola del sistema (winspool en Windows, CUPS en Linux/macOS) |
| `"printer://EPSON TM-T20"` | Lo mismo, forzado (útil si el nombre parece un puerto) |
| `"tcp://192.168.1.50:9100"` | Red. Si omites el puerto se usa 9100 |
| `"192.168.1.50:9100"` | Red (forma corta) |
| `"COM3"` / `"com://COM3"` | Puerto serie en Windows |
| `"/dev/ttyUSB0"` | Puerto serie en Linux / macOS |
| `"bt://COM5"` / `"bt://rfcomm0"` | Bluetooth ya emparejada |
| `"device:///dev/usb/lp0"` | Impresora USB directa en Linux |
| `"caja"` | Impresora dada de alta (ver abajo) |

Una impresora dada de alta puede apuntar a varios destinos separados por
coma: se encola una copia para cada uno.

**Bluetooth:** el emparejado lo hace el sistema operativo, no el agente. En
Windows, empareja la impresora y mira qué puerto COM saliente le asigna
(`bt://COM5`). En Linux, empareja con `bluetoothctl` y crea el nodo con
`sudo rfcomm bind 0 AA:BB:CC:DD:EE:FF` (`bt://rfcomm0`).

**USB:** en Windows se direcciona por el nombre de la impresora en el
sistema; `USB001` es un puerto del spooler, no un destino. En Linux puedes
usar `device:///dev/usb/lp0` directamente.

## Impresoras dadas de alta

En vez de repetir el ancho de papel y el modo de corte en cada llamada, das
de alta cada impresora una vez con sus ajustes. Desde el panel, pestaña
**Impresoras**, o con `POST /api/printers-config`:

```json
[
  {
    "name": "caja",
    "target": "tcp://192.168.1.50:9100",
    "paper_width": 576,
    "cut": "partial",
    "description": "Caja principal 80 mm"
  },
  {
    "name": "cocina",
    "target": "EPSON Cocina,EPSON Barra",
    "paper_width": 384,
    "cut": "none",
    "upside_down": true,
    "font": "b",
    "description": "Comandas, sale en las dos"
  }
]
```

Después basta con `{"printer": "caja", ...}` y el ticket sale a 80 mm con
corte parcial. Lo que mandes en la petición manda sobre el perfil.

Un nombre que no esté dado de alta se usa tal cual, así que no hace falta
configurar nada para empezar.

## Opciones de documento

Valen para **todos** los endpoints de impresión:

| Campo | Qué hace |
|---|---|
| `width` | Ancho del papel en puntos: 384 (58 mm), 512 (72 mm), 576 (80 mm) |
| `cut` | `"partial"`, `"full"`, `"none"`. También acepta `true`/`false` |
| `compact` | `true` aprieta el interlineado: ~20% menos papel por ticket |
| `line_spacing` | Alto de línea exacto en puntos. 0 deja el de la impresora |
| `upside_down` | `true` imprime el ticket girado 180° |
| `feed_top` | Líneas en blanco antes del contenido. **0 por defecto** |
| `feed_bottom` | Líneas que se avanzan al cortar. Mínimo 4 |
| `margin_dots` | Margen izquierdo en puntos |
| `font` | `"a"` normal, `"b"` condensada (entra más texto por línea) |
| `drawer` | `true` abre el cajón de dinero |

> **Espacio arriba:** el agente no emite ni un salto de línea antes del
> contenido. Si ves papel en blanco al principio, viene de la distancia
> física entre la cuchilla y el cabezal del ticket anterior; baja
> `feed_bottom` para reducirla.

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

### Elementos de `lines`

El campo `type` decide qué se dibuja. Un ticket se describe de arriba abajo
en un solo array.

| `type` | Campo que usa | Qué dibuja |
|---|---|---|
| vacío o `"text"` | `text` | Una línea de texto |
| `"table"` | `table` | Una tabla |
| `"qr"` | `qr` | Un código QR |
| `"barcode"` | `barcode` | Un código de barras |
| `"image"` | `image` | Una imagen en base64 |
| `"rule"` | `rule` | Una línea separadora del ancho del papel |
| `"feed"` | `feed` | Líneas en blanco |

Formato, aplicable a cualquier elemento:

| Campo | Qué hace |
|---|---|
| `align` | `left`, `center` o `right` |
| `bold` / `underline` | Negrita / subrayado |
| `invert` | Blanco sobre negro |
| `size` | `normal`, `small`, `double`, `wide`, `tall` |
| `scale_w` / `scale_h` | Multiplicador exacto de 1 a 8. Manda sobre `size` |
| `box` | Enmarca el texto en un recuadro |
| `gap` | Líneas en blanco después |
| `ml` / `mr` | Margen izquierdo / derecho, en caracteres |

### QR

```json
{ "type": "qr", "qr": { "data": "https://...", "size": 6, "ec": "M" } }
```

`size` es el lado de cada punto, de 1 a 16. **Si lo omites se calcula solo**
según el ancho del papel y lo que ocupe el contenido, de forma que un QR
largo no se salga del papel. `ec` es la corrección de errores: `L`, `M`
(por defecto), `Q` o `H` — más corrección significa un QR más grande pero
legible aunque se manche.

Máximo 2953 caracteres.

### Código de barras

```json
{ "type": "barcode",
  "barcode": { "data": "123456789", "type": "code128", "height": 60,
               "width": 3, "hri": "below" } }
```

`type`: `code128` (por defecto), `ean13`, `ean8`, `upca`, `upce`, `code39`,
`code93`, `itf`, `codabar`, `pdf417`. `height` en puntos (1-255). `width` es
el grosor de la barra fina (2-6); **si lo omites se calcula** para que quepa
en el papel. `hri` es dónde va el texto legible: `none`, `above`, `below`
(por defecto) o `both`.

Máximo 253 caracteres.

### Imagen

```json
{ "type": "image", "image": "data:image/png;base64,iVBORw0KGgo..." }
```

### Atajos

`qr` y `barcode` a nivel raíz siguen funcionando y equivalen a un elemento
al final del ticket.

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

El agente **no rasteriza la página**: traduce el HTML a comandos de texto de
la impresora, así que el ticket sale nítido y rápido. Eso marca el límite de
lo que se entiende, que es el formato que cabe en un ticket.

**Etiquetas**

| Grupo | Etiquetas |
|---|---|
| Bloques | `h1`-`h6`, `p`, `div`, `section`, `header`, `footer`, `center`, `pre` |
| Texto | `b`/`strong`, `i`/`em`/`u`/`ins`, `small`, `big`, `mark` (invertido), `span`, `font` |
| Listas | `ul`, `ol`, `li` (con viñetas y numeración, anidables) |
| Tablas | `table`, `tr`, `td`, `th` (cabecera en negrita) |
| Separadores | `br`, `hr` |
| Imágenes | `img` con `src="data:image/png;base64,..."` |
| Propias | `qr`, `barcode`, `feed` |

**CSS en línea** (atributo `style`)

| Propiedad | Se traduce a |
|---|---|
| `text-align: left\|center\|right` | Alineación de la línea |
| `font-weight: bold` | Negrita |
| `text-decoration: underline` | Subrayado |
| `font-size` | Escala del texto: `small` → condensada, `large` → 2x, `x-large` → 3x, `xx-large` → 4x. También acepta píxeles |

**Etiquetas propias**

```html
<qr data="https://kollatek.com" size="6" ec="M"></qr>
<barcode data="123456789" type="code128" height="60" hri="below"></barcode>
<feed lines="2">
<hr char="=">
```

**Detalles que conviene saber**

- Los saltos de línea del fuente **no** son saltos del ticket: usa `<br>` o
  bloques. Dentro de `<pre>` sí se respetan.
- El contenido de `<style>`, `<script>`, `<title>` y `<head>` se ignora.
- En una tabla, **la última columna se alinea sola a la derecha**: en un
  ticket suele ser el importe. El atributo `width` de `<td>` fija el ancho
  de esa columna en caracteres.
- Una imagen que no se pueda leer se salta sin romper el resto del ticket.

Hay ejemplos completos en [examples/html/](examples/html/).

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
| `GET /api/support-bundle` | **Un .zip con todo lo necesario para soporte**: diagnóstico, configuración, impresoras, cola y los últimos 7 días de registro, con el token tapado |
| `GET /api/logs?limit=80` | Últimas líneas del log (máximo 500) |
| `GET /api/token` | Consultar el token de acceso |
| `POST /api/settings` | Cambiar ancho de papel y escala |
| `GET`+`POST /api/printers-config` | Impresoras dadas de alta y sus ajustes |
| `POST /api/printer-aliases` | Compatibilidad: solo nombre y destino |

## Alias de impresora

```json
[
  { "name": "cocina", "printer": "EPSON Cocina,EPSON Barra", "description": "Pedidos a cocina y barra" },
  { "name": "facturas", "printer": "EPSON Caja", "description": "Facturacion" }
]
```

Enviándolo a `POST /api/printer-aliases`, tu sistema puede luego imprimir con
`{"printer": "cocina", ...}` y sale una copia en cada impresora del alias.
