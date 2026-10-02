# collatech-sdk

SDK oficial para **CollaTech Agent**. Imprime recibos, facturas, tickets, códigos QR y barras desde cualquier app JavaScript/TypeScript.

## Instalación

```bash
npm install collatech-sdk
```

```bash
yarn add collatech-sdk
```

```bash
pnpm add collatech-sdk
```

## Requisitos

- Node.js ≥ 18 (usa `fetch` nativo) o cualquier navegador moderno
- [CollaTech Agent](https://github.com/collatech/agent) corriendo en `localhost:18743`

## Inicio rápido

```ts
import { CollaTech } from "collatech-sdk";

const printer = new CollaTech();

// Verificar conexión
await printer.health();

// Imprimir texto
await printer.printText({
  printer: "Impresora1",
  text: "Hola Mundo!",
});
```

## Configuración

```ts
const printer = new CollaTech({
  baseUrl: "http://localhost:18743", // default
  timeout: 15000,                    // 15 segundos
  debug: false,                      // true para ver requests en consola
});
```

## Uso por framework

### React / Next.js

```tsx
"use client";

import { CollaTech } from "collatech-sdk";

const printer = new CollaTech();

export function PrintButton() {
  const handlePrint = async () => {
    await printer.printText({
      printer: "Impresora1",
      text: "Ticket desde React",
    });
  };

  return <button onClick={handlePrint}>Imprimir</button>;
}
```

### Angular

```typescript
import { CollaTech } from "collatech-sdk";

@Component({ /* ... */ })
export class PrintComponent {
  private printer = new CollaTech();

  async onPrint() {
    await this.printer.printText({
      printer: "cocina",
      text: "Ticket desde Angular",
    });
  }
}
```

Puedes configurar alias por estacion en el panel del agente: `cocina`, `recepcion`, `facturas`, `pagos`. Angular envia el alias y el agente lo resuelve a una o varias impresoras reales. Ejemplo: `cocina=EPSON Cocina,EPSON Barra`.

### Vue / Nuxt

```vue
<script setup lang="ts">
import { CollaTech } from "collatech-sdk";

const printer = new CollaTech();

async function handlePrint() {
  await printer.printText({
    printer: "Impresora1",
    text: "Ticket desde Vue",
  });
}
</script>

<template>
  <button @click="handlePrint">Imprimir</button>
</template>
```

### Node.js / Express

```js
import { CollaTech } from "collatech-sdk";

const printer = new CollaTech();

app.post("/api/print-factura", async (req, res) => {
  await printer.printTemplate({
    printer: "Impresora1",
    template: "recibo",
    data: {
      empresa: "Mi Empresa",
      cliente: req.body.cliente,
      total: req.body.total,
    },
  });
  res.json({ ok: true });
});
```

## API Reference

### Métodos principales

| Método | Descripción |
|--------|-------------|
| `health()` | Verificar conexión |
| `status()` | Estado y cola de trabajos |
| `printers()` | Listar impresoras |
| `printerAliases()` | Listar estaciones/alias |
| `savePrinterAliases(aliases)` | Guardar estaciones/alias |
| `templates()` | Listar templates disponibles |
| `getSettings()` | Obtener configuración |
| `saveSettings(settings)` | Guardar configuración |
| `logs(limit?)` | Obtener logs |
| `printText(opts)` | Imprimir texto plano |
| `printTicket(opts)` | Imprimir ticket estructurado |
| `printTemplate(opts)` | Imprimir con template |
| `printHTML(opts)` | Imprimir HTML custom |
| `printImage(opts)` | Imprimir imagen |
| `printLogo(opts)` | Imprimir logo por defecto |
| `printRaw(opts)` | Enviar ESC/POS raw |
| `createJob(printer)` | Crear builder fluente |

---

### Funciones nuevas para Angular / POS

```ts
await printer.waitUntilReady({ retries: 10, intervalMs: 500 });

await printer.configurePrinterAliases([
  { name: "cocina", printer: "EPSON Cocina,EPSON Barra", description: "Pedidos" },
  { name: "recepcion", printer: "EPSON Caja", description: "Caja" },
  { name: "facturas", printer: "EPSON Facturas", description: "Facturas" },
  { name: "pagos", printer: "EPSON Caja", description: "Pagos" },
]);

await printer.printToStation("cocina", "Pedido #1001\nMesa 4");
await printer.testStation("cocina", 1);
await printer.openDrawer("recepcion");
```

Helpers disponibles para integrar rapido:

| Metodo | Uso |
|--------|-----|
| `isReady()` | Saber si el agente esta corriendo |
| `waitUntilReady(opts?)` | Esperar antes de imprimir |
| `network()` | Obtener rutas LAN del agente |
| `configurePrinterAliases(aliases)` | Configurar cocina, caja, facturas, pagos |
| `printToStation(station, text)` | Imprimir texto usando alias |
| `testStation(station, copies?)` | Mandar prueba rapida |
| `printInvoice(opts)` | Imprimir factura con template |
| `printReceipt(opts)` | Imprimir recibo con template |
| `openDrawer(printer)` | Abrir caja |

Factura y recibo rapidos:

```ts
await printer.printInvoice({
  printer: "facturas",
  empresa: "Mi Tienda",
  cliente: "Cliente Demo",
  items: [
    { nombre: "Producto A", precio: "Bs 50.00" },
    { nombre: "Producto B", precio: "Bs 70.00" },
  ],
  total: "Bs 120.00",
  mensaje: "Gracias por su compra",
});

await printer.printReceipt({
  printer: "pagos",
  empresa: "Mi Tienda",
  cliente: "Cliente Demo",
  total: "Bs 120.00",
  mensaje: "Pago recibido",
});
```

### printText

```ts
await printer.printText({
  printer: "cocina",
  text: "Hola Mundo",
  cut: true,  // default: true
});
```

### printTicket

```ts
await printer.printTicket({
  printer: "Impresora1",
  title: "Mi Tienda",
  lines: [
    { text: "Cafe", align: "left" },
    { text: "Bs 5.00", align: "right", bold: true },
  ],
  qr: "https://example.com",
  barcode: "123456789",
  cut: true,
});
```

### printTemplate

Templates disponibles: `recibo`, `comanda`, `texto`, `qr`, `imagen`, o cualquier otro para factura por defecto.

```ts
await printer.printTemplate({
  printer: "Impresora1",
  template: "recibo",
  data: {
    empresa: "Mi Empresa C.A.",
    cliente: "Juan Perez",
    total: "Bs 250.00",
    items: [
      { nombre: "Producto A", precio: "Bs 100.00" },
      { nombre: "Producto B", precio: "Bs 150.00" },
    ],
    qr: "https://pago.example.com/factura/123",
  },
});
```

### printHTML

Renderiza HTML directamente a comandos ESC/POS nativos (rapido, sin imagen).
Soporta: bold, underline, center, tables, hr, headings.

```ts
await printer.printHTML({
  printer: "Impresora1",
  html: `
    <center><b>MI EMPRESA</b></center>
    <hr>
    <p><b>Factura #00123</b></p>
    <p>Fecha: 2026-05-29</p>
    <hr>
    <table>
      <tr><td>Cafe</td><td style="text-align:right">Bs 5.00</td></tr>
      <tr><td>Te</td><td style="text-align:right">Bs 3.00</td></tr>
    </table>
    <hr>
    <p style="text-align:right"><b>TOTAL: Bs 8.00</b></p>
    <hr>
    <center>Gracias por su compra!</center>
  `,
  cut: true,
});
```

### printImage

```ts
import { fileToBase64 } from "collatech-sdk";

// Desde el navegador con un input file
const base64 = await fileToBase64(fileInput.files[0]);
await printer.printImage({
  printer: "Impresora1",
  image: base64,
  width: 48,
  cut: true,
});
```

### printRaw (ESC/POS)

```ts
await printer.printRaw({
  printer: "Impresora1",
  data: "GUFjQUFBSUFBQUFB",  // base64 de bytes ESC/POS
  base64: true,
});
```

### Builder fluente

```ts
await printer
  .createJob("Impresora1")
  .title("Mi Tienda")
  .boldLine("Producto A")
  .line("Bs 100.00", { align: "right" })
  .line("─".repeat(32))
  .boldLine("TOTAL: Bs 100.00", { align: "right" })
  .qr("https://example.com/pago/123")
  .cut(true)
  .print();
```

## Manejo de errores

```ts
import {
  CollaTech,
  ConnectionError,
  ApiError,
  ForbiddenError,
} from "collatech-sdk";

const printer = new CollaTech();

try {
  await printer.printText({ printer: "Impresora1", text: "Test" });
} catch (err) {
  if (err instanceof ConnectionError) {
    console.error("El agente no está corriendo");
  } else if (err instanceof ForbiddenError) {
    console.error("Acceso denegado – solo localhost");
  } else if (err instanceof ApiError) {
    console.error(`Error ${err.status}: ${err.apiMessage}`);
  }
}
```

## Utilidades

```ts
import { fileToBase64, toBase64, fromBase64, stripDataUri } from "collatech-sdk";

// File → base64 (para imágenes)
const base64 = await fileToBase64(file);

// ArrayBuffer → base64
const buf = await file.arrayBuffer();
const b64 = toBase64(buf);

// base64 → Uint8Array
const bytes = fromBase64(b64);

// Strip data-URI prefix
const raw = stripDataUri("data:image/png;base64,iVBOR...");
```

## Licencia

MIT
