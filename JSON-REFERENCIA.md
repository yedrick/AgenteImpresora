# Formato JSON para imprimir

## 1. TEXTO

```json
{
  "printer": "Impresora1",
  "type": "text",
  "text": "Hola Mundo",
  "cut": true
}
```

## 2. TICKET

```json
{
  "printer": "Impresora1",
  "type": "ticket",
  "title": "MI TIENDA",
  "lines": [
    { "text": "Factura #001", "align": "center", "bold": true },
    { "text": "--------------------------------" },
    { "text": "Cafe ............ Bs 5.00" },
    { "text": "Te .............. Bs 3.00" },
    { "text": "--------------------------------" },
    { "text": "TOTAL: Bs 8.00", "align": "right", "bold": true }
  ],
  "qr": "https://pago.com/123",
  "barcode": "123456789",
  "cut": true
}
```

### Lineas con formato:

```json
{
  "text": "Texto aqui",
  "align": "left|center|right",
  "bold": true|false,
  "underline": true|false
}
```

## 3. HTML

```json
{
  "printer": "Impresora1",
  "type": "html",
  "html": "<center><b>MI EMPRESA</b></center><hr><p>Total: Bs 10</p>",
  "cut": true
}
```

### HTML soportado:

```html
<center>Centrado</center>
<b>Negrita</b>
<u>Subrayado</u>
<p>Parrafo</p>
<p style="text-align:right">Derecha</p>
<hr>Linea separadora
<table>
  <tr><td>Izquierda</td><td style="text-align:right">Derecha</td></tr>
</table>
```

## 4. TEMPLATE

```json
{
  "printer": "Impresora1",
  "type": "template",
  "template": "recibo",
  "templateData": {
    "empresa": "Mi Tienda",
    "cliente": "Juan Perez",
    "total": "Bs 100",
    "items": [
      { "nombre": "Cafe", "precio": "Bs 5" }
    ]
  },
  "cut": true
}
```

### Templates disponibles:

- `recibo` - Recibo de venta
- `comanda` - Comanda de cocina
- `texto` - Texto simple
- `qr` - Codigo QR
- `imagen` - Logo/imagen
- `factura` - Factura (default)

## 5. IMAGEN

```json
{
  "printer": "Impresora1",
  "type": "image",
  "image": "data:image/png;base64,iVBORw0KGgo...",
  "cut": true
}
```

## 6. LOGO

```json
{
  "printer": "Impresora1",
  "type": "logo",
  "cut": true
}
```

## 7. RAW ESC/POS

```json
{
  "printer": "Impresora1",
  "type": "raw",
  "rawData": "GUFjQUFBSUFBQUFB",
  "rawBase64": true
}
```

---

## Ejemplo completo con todo

```json
{
  "printer": "Impresora1",
  "type": "ticket",
  "title": "MI EMPRESA C.A.",
  "lines": [
    { "text": "FACTURA #001234", "align": "center", "bold": true },
    { "text": "Fecha: 2026-05-30", "align": "left" },
    { "text": "Cliente: Juan Perez", "align": "left" },
    { "text": "--------------------------------" },
    { "text": "Laptop HP            Bs 1.200", "align": "left" },
    { "text": "Mouse                Bs   50", "align": "left" },
    { "text": "Teclado              Bs   80", "align": "left" },
    { "text": "--------------------------------" },
    { "text": "TOTAL: Bs 1.330", "align": "right", "bold": true, "underline": true }
  ],
  "qr": "https://factura.com/001234",
  "barcode": "0012345678901",
  "cut": true
}
```
