# Integracion CollaTech Agent SDK con Angular

## Recomendado: servicio listo para copiar

Usa este archivo como base:

```text
examples/angular/collatech-print.service.ts
(en la carpeta de entrega al cliente: ENTREGA_CLIENTE/angular/collatech-print.service.ts)
```

Copialo a:

```text
src/app/services/collatech-print.service.ts
```

Incluye funciones listas para:

- verificar si el agente esta listo
- listar impresoras
- configurar estaciones `cocina`, `recepcion`, `facturas`, `pagos`
- imprimir cocina
- imprimir factura
- imprimir recibo/pago
- abrir caja
- ver ultimos jobs/logs

Ejemplo:

```typescript
await this.print.configureStorePrinters({
  cocina: 'EPSON Cocina,EPSON Barra',
  recepcion: 'EPSON Caja',
  facturas: 'EPSON Facturas',
  pagos: 'EPSON Caja',
});

await this.print.printKitchen({
  numero: '1001',
  mesa: '4',
  items: [{ nombre: 'Hamburguesa', cantidad: 2, nota: 'Sin cebolla' }],
});
```

## Paso 1: Crear proyecto Angular

```bash
ng new mi-tienda
cd mi-tienda
```

## Paso 2: Instalar el SDK

Lo mas simple es instalar el paquete empaquetado que viene en la entrega:

```bash
npm install ./collatech-sdk-1.2.0.tgz
```

Esta en `ENTREGA_CLIENTE/angular/collatech-sdk-1.2.0.tgz`. Copialo a la raiz
de tu proyecto Angular antes de ejecutar el comando.

### Alternativa: instalar desde el codigo fuente

Si copias la carpeta `collatech-sdk/` dentro de tu proyecto, **hay que
compilarla primero**: `npm install ./collatech-sdk` instala lo que haya en
`dist/`, que no se versiona y puede estar vacio o desactualizado.

```bash
cd collatech-sdk
npm install
npm run build
cd ..
npm install ./collatech-sdk
```

## Paso 3: Crear el servicio de impresion

```bash
ng generate service services/printer
```

Edita `src/app/services/printer.service.ts`:

```typescript
import { Injectable } from '@angular/core';
import { CollaTech, CollaTechConfig } from 'collatech-sdk';

@Injectable({
  providedIn: 'root'
})
export class PrinterService {
  private client: CollaTech;

  constructor() {
    this.client = new CollaTech({
      baseUrl: 'http://localhost:18743',
      timeout: 10000,
      // Solo si tu app corre en OTRA PC distinta a la del agente. El token lo
      // muestra el instalador al terminar y el panel del agente (Estado).
      // token: 'el-token-del-agente',
    });
  }

  // Verificar conexion
  async health() {
    return this.client.health();
  }

  // Listar impresoras
  async getPrinters() {
    return this.client.printers();
  }

  // Imprimir ticket
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
    cut?: boolean;
  }) {
    return this.client.printTicket(data);
  }

  // Imprimir con template
  async printTemplate(data: {
    printer: string;
    template: string;
    data?: Record<string, any>;
    cut?: boolean;
  }) {
    return this.client.printTemplate(data);
  }

  // Imprimir HTML nativo ESC/POS
  async printHTML(data: {
    printer: string;
    html: string;
    cut?: boolean;
  }) {
    return this.client.printHTML(data);
  }

  // Imprimir texto
  async printText(printer: string, text: string) {
    return this.client.printText({ printer, text });
  }

  // Builder fluente
  createJob(printer: string) {
    return this.client.createJob(printer);
  }
}
```

## Paso 4: Crear modulo de impresion (opcional)

```bash
ng generate module printing
ng generate component printing/print-panel
```

Edita `src/app/printing/print-panel/print-panel.component.ts`:

```typescript
import { Component, OnInit } from '@angular/core';
import { PrinterService } from '../../services/printer.service';

@Component({
  selector: 'app-print-panel',
  templateUrl: './print-panel.component.html',
  styleUrls: ['./print-panel.component.css']
})
export class PrintPanelComponent implements OnInit {
  printers: any[] = [];
  selectedPrinter = '';
  status = '';
  loading = false;

  constructor(private printer: PrinterService) {}

  async ngOnInit() {
    try {
      this.printers = await this.printer.getPrinters();
      if (this.printers.length > 0) {
        this.selectedPrinter = this.printers[0].name;
      }
      this.status = 'Conectado';
    } catch (e) {
      this.status = 'Error: Agente no disponible';
    }
  }

  async printTest() {
    if (!this.selectedPrinter) return;
    this.loading = true;
    try {
      await this.printer.printTicket({
        printer: this.selectedPrinter,
        title: 'MI TIENDA',
        lines: [
          { text: 'Factura #001', align: 'center', bold: true },
          { text: '--------------------------------' },
          { text: 'Cafe ............ Bs 5.00' },
          { text: 'Te .............. Bs 3.00' },
          { text: '--------------------------------' },
          { text: 'TOTAL: Bs 8.00', align: 'right', bold: true },
        ],
        cut: true,
      });
      this.status = 'Impresion enviada!';
    } catch (e: any) {
      this.status = `Error: ${e.message}`;
    }
    this.loading = false;
  }

  async printFactura() {
    if (!this.selectedPrinter) return;
    this.loading = true;
    try {
      await this.printer.printHTML({
        printer: this.selectedPrinter,
        html: `
          <center><b>MI EMPRESA C.A.</b></center>
          <center>RIF: J-12345678-9</center>
          <hr>
          <p><b>FACTURA #001234</b></p>
          <p>Fecha: ${new Date().toLocaleDateString()}</p>
          <p>Cliente: Juan Perez</p>
          <hr>
          <table>
            <tr><td>Cafe x2</td><td style="text-align:right">Bs 10.00</td></tr>
            <tr><td>Te x1</td><td style="text-align:right">Bs 3.00</td></tr>
            <tr><td>Jugo x3</td><td style="text-align:right">Bs 12.00</td></tr>
          </table>
          <hr>
          <p style="text-align:right"><b>TOTAL: Bs 25.00</b></p>
          <hr>
          <center>Gracias por su compra!</center>
        `,
        cut: true,
      });
      this.status = 'Factura enviada!';
    } catch (e: any) {
      this.status = `Error: ${e.message}`;
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
          empresa: 'Mi Tienda C.A.',
          cliente: 'Maria Lopez',
          total: 'Bs 250.00',
          mensaje: 'Gracias por su compra!',
        },
        cut: true,
      });
      this.status = 'Recibo enviado!';
    } catch (e: any) {
      this.status = `Error: ${e.message}`;
    }
    this.loading = false;
  }
}
```

Edita `src/app/printing/print-panel/print-panel.component.html`:

```html
<div class="print-panel">
  <h2>Panel de Impresion</h2>

  <div class="status" [class.error]="status.includes('Error')">
    {{ status }}
  </div>

  <div class="form-group">
    <label>Impresora:</label>
    <select [(ngModel)]="selectedPrinter">
      <option *ngFor="let p of printers" [value]="p.name">
        {{ p.name }} ({{ p.type }})
      </option>
    </select>
  </div>

  <div class="buttons">
    <button (click)="printTest()" [disabled]="loading || !selectedPrinter">
      Imprimir Ticket
    </button>

    <button (click)="printFactura()" [disabled]="loading || !selectedPrinter">
      Imprimir Factura HTML
    </button>

    <button (click)="printRecibo()" [disabled]="loading || !selectedPrinter">
      Imprimir Recibo Template
    </button>
  </div>
</div>
```

Edita `src/app/printing/print-panel/print-panel.component.css`:

```css
.print-panel {
  max-width: 500px;
  margin: 20px auto;
  padding: 20px;
  border: 1px solid #ccc;
  border-radius: 8px;
}

.status {
  padding: 10px;
  margin-bottom: 15px;
  background: #e8f5e9;
  border-radius: 4px;
  text-align: center;
}

.status.error {
  background: #ffebee;
  color: #c62828;
}

.form-group {
  margin-bottom: 15px;
}

.form-group label {
  display: block;
  margin-bottom: 5px;
  font-weight: bold;
}

.form-group select {
  width: 100%;
  padding: 8px;
  border: 1px solid #ccc;
  border-radius: 4px;
}

.buttons {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.buttons button {
  padding: 12px;
  border: none;
  border-radius: 4px;
  background: #1976d2;
  color: white;
  font-size: 14px;
  cursor: pointer;
}

.buttons button:hover:not(:disabled) {
  background: #1565c0;
}

.buttons button:disabled {
  background: #ccc;
  cursor: not-allowed;
}
```

## Paso 5: Importar modulo en app.module.ts

```typescript
import { NgModule } from '@angular/core';
import { BrowserModule } from '@angular/platform-browser';
import { FormsModule } from '@angular/forms';

import { AppComponent } from './app.component';
import { PrintingModule } from './printing/printing.module';

@NgModule({
  declarations: [
    AppComponent,
  ],
  imports: [
    BrowserModule,
    FormsModule,
    PrintingModule,
  ],
  providers: [],
  bootstrap: [AppComponent]
})
export class AppModule { }
```

## Paso 6: Usar el componente

Edita `src/app/app.component.html`:

```html
<app-print-panel></app-print-panel>
```

## Paso 7: Configurar proxy (CORS)

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

## Paso 8: Ejecutar

```bash
ng serve
```

Abre `http://localhost:4200` y veras el panel de impresion.

---

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

## Estructura final del proyecto

```
mi-tienda/
├── src/
│   ├── app/
│   │   ├── services/
│   │   │   └── printer.service.ts
│   │   ├── printing/
│   │   │   ├── printing.module.ts
│   │   │   └── print-panel/
│   │   │       ├── print-panel.component.ts
│   │   │       ├── print-panel.component.html
│   │   │       └── print-panel.component.css
│   │   ├── app.component.html
│   │   └── app.module.ts
│   └── ...
├── collatech-sdk/
├── proxy.conf.json
├── package.json
└── angular.json
```
