# Integracion CollaTech SDK + Angular

## Integracion rapida recomendada

Ya existe un servicio listo para copiar:

```text
examples/angular/collatech-print.service.ts
(en la carpeta de entrega al cliente: ENTREGA_CLIENTE/angular/collatech-print.service.ts)
```

Copialo a tu proyecto Angular:

```text
src/app/services/collatech-print.service.ts
```

Instala el SDK (en la entrega va empaquetado como `.tgz`):

```bash
npm install ./collatech-sdk-1.5.0.tgz
```

Uso rapido desde un componente:

```typescript
import { Component } from '@angular/core';
import { CollaTechPrintService } from './services/collatech-print.service';

@Component({
  selector: 'app-pos',
  template: `
    <button (click)="probar()">Probar cocina</button>
    <button (click)="factura()">Factura</button>
    <button (click)="caja()">Abrir caja</button>
  `,
})
export class PosComponent {
  constructor(private print: CollaTechPrintService) {}

  async probar() {
    await this.print.testStation('cocina', 1);
  }

  async factura() {
    await this.print.printInvoice({
      empresa: 'Mi Tienda',
      cliente: 'Cliente Demo',
      items: [
        { nombre: 'Producto A', precio: 'Bs 50.00' },
        { nombre: 'Producto B', precio: 'Bs 70.00' },
      ],
      total: 'Bs 120.00',
      mensaje: 'Gracias por su compra',
    });
  }

  async caja() {
    await this.print.openCashDrawer();
  }
}
```

Configurar estaciones desde Angular:

```typescript
await this.print.configureStorePrinters({
  cocina: 'EPSON Cocina,EPSON Barra',
  recepcion: 'EPSON Caja',
  facturas: 'EPSON Facturas',
  pagos: 'EPSON Caja',
});
```

Con esto tu app imprime usando alias (`cocina`, `facturas`, `pagos`) y no depende del nombre fisico en cada pantalla.

## Paso 1: Crear proyecto Angular

```bash
ng new mi-tienda
cd mi-tienda
```

## Paso 2: Copiar SDK a tu proyecto

Copia `collatech-sdk-1.5.0.tgz` (esta en la carpeta `angular/` de la entrega)
dentro de `mi-tienda/`:

```
mi-tienda/
├── collatech-sdk-1.5.0.tgz    <-- COPIAR AQUI
├── src/
├── angular.json
└── package.json
```

## Paso 3: Instalar dependencias

```bash
npm install ./collatech-sdk-1.5.0.tgz
```

Queda en tu `package.json` como `"collatech-sdk": "file:collatech-sdk-1.5.0.tgz"`.
Al pasar a una version nueva, copia el `.tgz` nuevo y repite el `npm install`.

## Paso 4: Crear servicio de impresion

```bash
ng generate service services/printer --skip-tests
```

## Paso 5: Editar el servicio

Reemplaza el contenido de `src/app/services/printer.service.ts`:

```typescript
import { Injectable } from '@angular/core';
import { CollaTech } from 'collatech-sdk';

@Injectable({ providedIn: 'root' })
export class PrinterService {
  private client: CollaTech;

  constructor() {
    this.client = new CollaTech({
      baseUrl: 'http://localhost:18743',
      // Solo hace falta si tu app NO corre en la misma PC que el agente.
      // Lo muestra el instalador al terminar, y el panel en la pestana Estado.
      token: (window as any).__COLLATECH_TOKEN__ || '',
    });
  }

  async health() {
    return this.client.health();
  }

  async printers() {
    return this.client.printers();
  }

  async printTicket(data: {
    printer: string;
    title?: string;
    lines?: Array<{
      text: string;
      align?: 'left' | 'center' | 'right';
      bold?: boolean;
      underline?: boolean;
    }>;
    qr?: string;
    barcode?: string;
    cut?: boolean;
  }) {
    return this.client.printTicket(data);
  }

  async printHTML(data: {
    printer: string;
    html: string;
    width?: 384 | 512 | 576;   // los tres anchos que entiende el agente
    cut?: boolean;
  }) {
    return this.client.printHTML(data);
  }

  async printTemplate(data: {
    printer: string;
    template: string;
    data?: Record<string, any>;
  }) {
    return this.client.printTemplate(data);
  }

  async printText(printer: string, text: string) {
    return this.client.printText({ printer, text, cut: true });
  }

  createJob(printer: string) {
    return this.client.createJob(printer);
  }
}
```

## Paso 6: Crear componente

```bash
ng generate component components/pos --skip-tests
```

## Paso 7: Editar el componente

Reemplaza `src/app/components/pos/pos.component.ts`:

```typescript
import { Component, OnInit } from '@angular/core';
import { PrinterService } from '../../services/printer.service';

@Component({
  selector: 'app-pos',
  templateUrl: './pos.component.html',
  styleUrls: ['./pos.component.css']
})
export class PosComponent implements OnInit {
  printers: any[] = [];
  selectedPrinter = '';
  status = '';
  loading = false;

  empresa = 'Mi Tienda C.A.';
  cliente = 'Cliente Demo';
  items = 'Cafe|Bs 5.00\nTe|Bs 3.00';
  total = 'Bs 8.00';
  qr = '';

  constructor(private printer: PrinterService) {}

  async ngOnInit() {
    try {
      await this.printer.health();
      this.printers = await this.printer.printers();
      if (this.printers.length > 0) {
        this.selectedPrinter = this.printers[0].name;
      }
      this.status = 'Conectado al agente';
    } catch {
      this.status = 'Agente no disponible';
    }
  }

  parseItems() {
    return this.items.split('\n')
      .filter(l => l.trim())
      .map(l => {
        const [nombre, precio] = l.split('|');
        return { nombre: nombre?.trim() || '', precio: precio?.trim() || '' };
      });
  }

  async printTicket() {
    if (!this.selectedPrinter) return;
    this.loading = true;
    this.status = 'Imprimiendo...';
    try {
      await this.printer.printTicket({
        printer: this.selectedPrinter,
        title: this.empresa,
        lines: [
          { text: 'Cliente: ' + this.cliente, align: 'left' },
          { text: '--------------------------------' },
          ...this.parseItems().map(i => ({
            text: `${i.nombre.padEnd(24)}${i.precio}`
          })),
          { text: '--------------------------------' },
          { text: `TOTAL: ${this.total}`, bold: true, align: 'right' },
        ],
        qr: this.qr || undefined,
        cut: true,
      });
      this.status = 'Enviado a impresora!';
    } catch (e: any) {
      this.status = 'Error: ' + (e.message || e.error || 'Desconocido');
    }
    this.loading = false;
  }

  async printHTML() {
    if (!this.selectedPrinter) return;
    this.loading = true;
    this.status = 'Imprimiendo...';
    try {
      const items = this.parseItems()
        .map(i => `<tr><td>${i.nombre}</td><td style="text-align:right">${i.precio}</td></tr>`)
        .join('\n');

      await this.printer.printHTML({
        printer: this.selectedPrinter,
        width: 576,
        cut: true,
        html: `
        <center><b>${this.empresa}</b></center>
        <hr>
        <p>Cliente: ${this.cliente}</p>
        <hr>
        <table>
          ${items}
        </table>
        <hr>
        <p style="text-align:right"><b>TOTAL: ${this.total}</b></p>
        <hr>
        <center>Gracias por su compra!</center>
      `,
      });
      this.status = 'Enviado a impresora!';
    } catch (e: any) {
      this.status = 'Error: ' + (e.message || e.error || 'Desconocido');
    }
    this.loading = false;
  }

  async printRecibo() {
    if (!this.selectedPrinter) return;
    this.loading = true;
    try {
      await this.printer.printTemplate({
        printer: this.selectedPrinter,
        template: 'recibo',
        data: {
          empresa: this.empresa,
          cliente: this.cliente,
          total: this.total,
          mensaje: 'Gracias por su compra!',
        },
      });
      this.status = 'Recibo enviado!';
    } catch (e: any) {
      this.status = 'Error: ' + (e.message || e.error || 'Desconocido');
    }
    this.loading = false;
  }
}
```

## Paso 8: Editar el template

Reemplaza `src/app/components/pos/pos.component.html`:

```html
<div class="pos-container">
  <h2>Panel de Impresion</h2>

  <div class="status-bar" [class.error]="status.includes('Error')">
    {{ status }}
  </div>

  <div class="form-row">
    <label>Impresora:</label>
    <select [(ngModel)]="selectedPrinter">
      <option *ngFor="let p of printers" [value]="p.name">
        {{ p.name }} ({{ p.type }})
      </option>
    </select>
  </div>

  <div class="form-row">
    <label>Empresa:</label>
    <input [(ngModel)]="empresa" placeholder="Nombre de tu empresa">
  </div>

  <div class="form-row">
    <label>Cliente:</label>
    <input [(ngModel)]="cliente" placeholder="Nombre del cliente">
  </div>

  <div class="form-row">
    <label>Items (uno por linea: nombre|precio):</label>
    <textarea [(ngModel)]="items" rows="4" placeholder="Cafe|Bs 5.00"></textarea>
  </div>

  <div class="form-row">
    <label>Total:</label>
    <input [(ngModel)]="total" placeholder="Bs 0.00">
  </div>

  <div class="form-row">
    <label>QR (opcional):</label>
    <input [(ngModel)]="qr" placeholder="https://pago.com/123">
  </div>

  <div class="buttons">
    <button (click)="printTicket()" [disabled]="loading || !selectedPrinter">
      Imprimir TICKET
    </button>
    <button (click)="printHTML()" [disabled]="loading || !selectedPrinter" class="secondary">
      Imprimir HTML
    </button>
    <button (click)="printRecibo()" [disabled]="loading || !selectedPrinter" class="secondary">
      Imprimir RECIBO
    </button>
  </div>
</div>
```

## Paso 9: Editar estilos

Reemplaza `src/app/components/pos/pos.component.css`:

```css
.pos-container {
  max-width: 500px;
  margin: 40px auto;
  padding: 24px;
  border: 1px solid #ddd;
  border-radius: 8px;
  background: #fafafa;
}

h2 {
  margin: 0 0 16px;
  color: #333;
}

.status-bar {
  padding: 10px;
  margin-bottom: 16px;
  background: #e8f5e9;
  border-radius: 4px;
  text-align: center;
  font-size: 14px;
}

.status-bar.error {
  background: #ffebee;
  color: #c62828;
}

.form-row {
  margin-bottom: 12px;
}

.form-row label {
  display: block;
  margin-bottom: 4px;
  font-weight: bold;
  font-size: 13px;
  color: #555;
}

.form-row input,
.form-row select,
.form-row textarea {
  width: 100%;
  padding: 8px 12px;
  border: 1px solid #ccc;
  border-radius: 4px;
  font-size: 14px;
}

.buttons {
  display: flex;
  gap: 8px;
  margin-top: 16px;
}

.buttons button {
  flex: 1;
  padding: 12px;
  border: none;
  border-radius: 4px;
  font-size: 14px;
  font-weight: bold;
  cursor: pointer;
  background: #1976d2;
  color: white;
}

.buttons button:hover:not(:disabled) {
  background: #1565c0;
}

.buttons button.secondary {
  background: #666;
}

.buttons button.secondary:hover:not(:disabled) {
  background: #555;
}

.buttons button:disabled {
  background: #ccc;
  cursor: not-allowed;
}
```

## Paso 10: Importar FormsModule

Edita `src/app/app.module.ts`:

```typescript
import { NgModule } from '@angular/core';
import { BrowserModule } from '@angular/platform-browser';
import { FormsModule } from '@angular/forms';

import { AppComponent } from './app.component';
import { PosComponent } from './components/pos/pos.component';

@NgModule({
  declarations: [
    AppComponent,
    PosComponent,
  ],
  imports: [
    BrowserModule,
    FormsModule,
  ],
  providers: [],
  bootstrap: [AppComponent]
})
export class AppModule { }
```

## Paso 11: Usar el componente

Edita `src/app/app.component.html`:

```html
<app-pos></app-pos>
```

## Paso 12: Ejecutar

```bash
ng serve
```

Abre `http://localhost:4200`

---

## Novedades de la 1.5.0

### El agente exige `Content-Type: application/json`

Es el unico cambio que rompe compatibilidad. **Si usas el SDK no tienes que
hacer nada**: ya lo manda en todas sus peticiones. Solo te afecta si ademas
llamabas a la API con `fetch` a mano; en ese caso una peticion sin esa
cabecera responde ahora `415`:

```typescript
// Antes colaba. Ahora no.
fetch('http://localhost:18743/api/print/text', { method: 'POST', body: json });

// Asi si:
fetch('http://localhost:18743/api/print/text', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: json,
});
```

El motivo es que un `POST` sin cabeceras es una *peticion simple* para el
navegador: no pide permiso al agente antes de enviarla. Cualquier web que
abriera el cajero podia imprimir y **abrir el cajon de dinero** desde una
pestana cualquiera. Exigir `application/json` obliga al navegador a pedir
permiso primero.

### Impresoras dadas de alta

Cada impresora guarda su ancho de papel, su modo de corte y si imprime
girada, asi que ya no hay que repetirlo en cada impresion:

```typescript
await this.print.savePrinterProfiles([
  {
    name: 'caja',                    // como la llamas al imprimir
    target: 'EPSON TM-T20',          // el nombre real en Windows, o tcp://192.168.1.50:9100
    paper_width: 576,                // 384 (58 mm), 512 o 576 (80 mm)
    cut: 'partial',
    model: 'epson-tm-t20',           // opcional: del catalogo del panel
  },
  {
    name: 'cocina',
    target: 'EPSON TM-T88',
    paper_width: 384,
    cut: 'none',
    upside_down: true,               // la impresora esta montada al reves
  },
]);

const impresoras = await this.print.printerProfiles();
```

`savePrinterProfiles()` solo responde desde la propia PC del agente: es
configuracion, no impresion.

### Bloques maquetados: el QR al costado

Una termica imprime linea a linea, asi que nativamente no se puede poner un
QR al lado de un texto: el comando de QR imprime y avanza el papel. Un
**bloque** describe esa zona como una rejilla de filas y columnas, y el
agente la compone como una sola imagen:

```typescript
await this.print.printLayout('caja', {
  padding: 6,
  rows: [
    {
      align: 'middle',
      cols: [
        { weight: 1, items: [{ text: 'Factura A-00142', bold: true }] },
        { dots: 150, align: 'right', items: [{ qr: 'https://pago/A-00142' }] },
      ],
    },
  ],
});
```

Una columna se mide con `weight` (reparte el sitio que sobra) o con `dots`
(puntos fijos). El servicio de ejemplo trae `printInvoiceWithQR()` ya
armado, con la tabla con bordes y el total resaltado.

Cuesta mas que el texto nativo —unos 14 KB frente a 200 bytes—, asi que
conviene usarlo solo en la zona que lo necesita y dejar el resto en texto.

### Vista previa sin gastar papel

La compone el mismo codigo que imprime, asi que lo que se ve es lo que sale:

```typescript
const png = await this.print.previewLayout(bloque);
this.urlPrevia = URL.createObjectURL(png);  // <img [src]="urlPrevia">
```

Acuerdate de `URL.revokeObjectURL()` al cambiar de previa, o el navegador
va acumulando los PNG en memoria.

El diseñador visual del panel (`http://localhost:18743/designer`) arma estos
bloques a golpe de raton y te da el JSON listo para pegar aqui.

### Leer el resultado de una impresion

Todo lo que imprime devuelve `PrintResult`, que es **un trabajo o una lista**:
una impresora puede tener varios destinos (`target: 'Caja,Respaldo'`) y
entonces sale una copia en cada uno, con un trabajo por copia.

```typescript
const r = await this.print.printLayout('caja', bloque);
const trabajos = Array.isArray(r) ? r : [r];
console.log(trabajos.map((t) => t.id));
```

Son `202 Accepted`: el agente **encola** y responde enseguida, no espera a que
salga el papel. El trabajo nace `Pending`. Para saber como acabo, consulta
`lastJobs()`, que es lo que hace el panel.

## Estructura final

```
mi-tienda/
├── collatech-sdk-1.5.0.tgz
├── src/
│   ├── app/
│   │   ├── services/
│   │   │   └── printer.service.ts
│   │   ├── components/
│   │   │   └── pos/
│   │   │       ├── pos.component.ts
│   │   │       ├── pos.component.html
│   │   │       └── pos.component.css
│   │   ├── app.component.html
│   │   └── app.module.ts
│   └── ...
├── angular.json
└── package.json
```

---

## Uso en cualquier componente

```typescript
import { PrinterService } from '../services/printer.service';

@Component({ /* ... */ })
export class MiComponent {
  constructor(private printer: PrinterService) {}

  async imprimir() {
    await this.printer.printTicket({
      printer: 'Impresora1',
      title: 'MI TIENDA',
      lines: [
        { text: 'Cafe', align: 'left' },
        { text: 'Bs 5.00', align: 'right', bold: true },
      ],
      cut: true,
    });
  }
}
```

---

## Opcional: proxy de Angular para evitar CORS

Crea `proxy.conf.json` en la raiz del proyecto:

```json
{
  "/api": {
    "target": "http://localhost:18743",
    "secure": false,
    "changeOrigin": true
  },
  "/health": {
    "target": "http://localhost:18743",
    "secure": false
  }
}
```

Edita `angular.json` > `serve` > `options`:

```json
"proxyConfig": "proxy.conf.json"
```

Y actualiza el servicio para usar proxy:

```typescript
this.client = new CollaTech({
  baseUrl: '',  // Usa proxy de Angular
  timeout: 10000,
});
```

## Uso directo sin componente (ejemplo rapido)

```typescript
import { Component } from '@angular/core';
import { PrinterService } from './services/printer.service';

@Component({
  selector: 'app-root',
  template: `
    <button (click)="imprimir()">Imprimir</button>
  `
})
export class AppComponent {
  constructor(private printer: PrinterService) {}

  async imprimir() {
    await this.printer.printTicket({
      printer: 'Impresora1',
      title: 'MI TIENDA',
      lines: [
        { text: 'Cafe', align: 'left' },
        { text: 'Bs 5.00', align: 'right', bold: true },
      ],
      cut: true,
    });
  }
}
```

---
