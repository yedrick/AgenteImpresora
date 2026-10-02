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

Instala el SDK:

```bash
npm install ./collatech-sdk
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

## PASO 1: Crear proyecto Angular

```bash
ng new mi-tienda
cd mi-tienda
```

## PASO 2: Copiar SDK a tu proyecto

Copia la carpeta `collatech-sdk` dentro de `mi-tienda/`:

```
mi-tienda/
├── collatech-sdk/    <-- COPIAR AQUI
├── src/
├── angular.json
└── package.json
```

## PASO 3: Instalar dependencias

```bash
npm install ./collatech-sdk
```

## PASO 4: Crear servicio de impresion

```bash
ng generate service services/printer --skip-tests
```

## PASO 5: Editar el servicio

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
    width?: number;
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

## PASO 6: Crear componente

```bash
ng generate component components/pos --skip-tests
```

## PASO 7: Editar el componente

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

## PASO 8: Editar el template

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

## PASO 9: Editar estilos

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

## PASO 10: Importar FormsModule

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

## PASO 11: Usar el componente

Edita `src/app/app.component.html`:

```html
<app-pos></app-pos>
```

## PASO 12: Ejecutar

```bash
ng serve
```

Abre `http://localhost:4200`

---

## Estructura final

```
mi-tienda/
├── collatech-sdk/
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
