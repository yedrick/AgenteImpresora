import { Injectable } from '@angular/core';
import {
  CollaTech,
  ConnectionError,
  LayoutBlock,
  PaperWidth,
  PrinterAlias,
  PrinterInfo,
  PrinterProfile,
  PrintInvoiceOptions,
  PrintJob,
  PrintResult,
} from 'collatech-sdk';

@Injectable({ providedIn: 'root' })
export class CollaTechPrintService {
  private readonly client = new CollaTech({
    baseUrl: (window as any).__COLLATECH_URL__ || 'http://localhost:18743',
    // El agente exige un token a las peticiones que NO vienen de su propia
    // PC. Si tu aplicacion corre en la misma maquina, dejalo vacio. Si corre
    // en otra, inyectalo desde el HTML sin recompilar:
    //   <script>window.__COLLATECH_TOKEN__ = "...";</script>
    // Lo muestra el instalador al terminar y el panel del agente (Estado).
    token: (window as any).__COLLATECH_TOKEN__ || '',
    timeout: 15000,
  });

  async ready(): Promise<boolean> {
    return this.client.waitUntilReady({ retries: 10, intervalMs: 500 });
  }

  async requireReady(): Promise<void> {
    const ok = await this.ready();
    if (!ok) {
      throw new ConnectionError('CollaTech Agent no esta disponible');
    }
  }

  async printers(): Promise<PrinterInfo[]> {
    await this.requireReady();
    return this.client.printers();
  }

  async aliases(): Promise<PrinterAlias[]> {
    await this.requireReady();
    return this.client.printerAliases();
  }

  async saveAliases(aliases: PrinterAlias[]): Promise<PrinterAlias[]> {
    await this.requireReady();
    return this.client.savePrinterAliases(aliases);
  }

  async configureStorePrinters(config: {
    cocina: string;
    recepcion: string;
    facturas: string;
    pagos: string;
  }): Promise<PrinterAlias[]> {
    return this.saveAliases([
      { name: 'cocina', printer: config.cocina, description: 'Pedidos de cocina' },
      { name: 'recepcion', printer: config.recepcion, description: 'Recepcion y caja' },
      { name: 'facturas', printer: config.facturas, description: 'Facturas' },
      { name: 'pagos', printer: config.pagos, description: 'Pagos y recibos' },
    ]);
  }

  async printKitchen(order: {
    numero: string;
    mesa?: string;
    cliente?: string;
    items: Array<{ nombre: string; cantidad?: number; nota?: string }>;
  }): Promise<PrintResult> {
    await this.requireReady();
    const lines = [
      { text: `Pedido #${order.numero}`, align: 'center' as const, bold: true },
      order.mesa ? { text: `Mesa: ${order.mesa}`, bold: true } : undefined,
      order.cliente ? { text: `Cliente: ${order.cliente}` } : undefined,
      { text: '--------------------------------' },
      ...order.items.flatMap((item) => [
        { text: `${item.cantidad || 1} x ${item.nombre}`, bold: true },
        item.nota ? { text: `Nota: ${item.nota}` } : undefined,
      ]),
    ].filter(Boolean) as Array<{ text: string; align?: 'left' | 'center' | 'right'; bold?: boolean }>;

    return this.client.printTicket({
      printer: 'cocina',
      title: 'COCINA',
      lines,
      cut: true,
    });
  }

  async printInvoice(data: Omit<PrintInvoiceOptions, 'printer'>): Promise<PrintResult> {
    await this.requireReady();
    return this.client.printInvoice({
      printer: 'facturas',
      ...data,
    });
  }

  async printReceipt(data: Omit<PrintInvoiceOptions, 'printer'>): Promise<PrintResult> {
    await this.requireReady();
    return this.client.printReceipt({
      printer: 'pagos',
      ...data,
    });
  }

  async printText(station: string, text: string): Promise<PrintResult> {
    await this.requireReady();
    return this.client.printToStation(station, text, true);
  }

  async openCashDrawer(): Promise<PrintResult> {
    await this.requireReady();
    return this.client.openDrawer('recepcion');
  }

  async testStation(station: string, copies = 1): Promise<PrintResult[]> {
    await this.requireReady();
    return this.client.testStation(station, copies);
  }

  async lastJobs(): Promise<PrintJob[]> {
    await this.requireReady();
    const status = await this.client.status();
    return status.jobs;
  }

  // --- Impresoras dadas de alta -------------------------------------------
  //
  // Cada impresora guarda su ancho de papel, modo de corte y giro, asi que
  // luego basta con nombrarla: puedes tener una de 58 mm en cocina y una de
  // 80 mm en caja sin repetirlo en cada impresion.

  async printerProfiles(): Promise<PrinterProfile[]> {
    await this.requireReady();
    return this.client.printerProfiles();
  }

  /** Solo responde desde la propia PC del agente. */
  async savePrinterProfiles(list: PrinterProfile[]): Promise<PrinterProfile[]> {
    await this.requireReady();
    return this.client.savePrinterProfiles(list);
  }

  // --- Bloque maquetado ------------------------------------------------------
  //
  // Una impresora ESC/POS imprime linea a linea, asi que nativamente no se
  // puede poner un QR al costado de un texto. Un bloque describe esa zona
  // como una rejilla de filas y columnas y el agente la compone como imagen.
  //
  // Cuesta mas que el texto nativo (unos 14 KB frente a 200 bytes), asi que
  // conviene usarlo solo en la zona que lo necesita.

  async printLayout(station: string, layout: LayoutBlock, width: PaperWidth = 576): Promise<PrintResult> {
    await this.requireReady();
    return this.client.printLayout({ printer: station, width, layout, cut: 'partial' });
  }

  /**
   * Devuelve la vista previa como Blob, compuesta por el mismo codigo que
   * imprime. Util para ensenarla antes de gastar papel:
   *   this.url = URL.createObjectURL(await svc.previewLayout(bloque));
   */
  async previewLayout(layout: LayoutBlock, width: PaperWidth = 576, scale = 2): Promise<Blob> {
    await this.requireReady();
    return this.client.previewLayout({ layout, width, scale });
  }

  /** Factura con los datos a la izquierda y el QR al costado. */
  async printInvoiceWithQR(station: string, data: {
    empresa: string;
    numero: string;
    cliente: string;
    items: Array<{ nombre: string; cantidad: string; precio: string }>;
    total: string;
    qr: string;
  }): Promise<PrintResult> {
    const layout: LayoutBlock = {
      padding: 6,
      rows: [
        { cols: [{ weight: 1, align: 'center', items: [
          { text: data.empresa, size: 'xl', bold: true },
        ] }] },
        { cols: [{ weight: 1, items: [{ type: 'rule', height: 2 }] }] },
        { align: 'middle', cols: [
          { weight: 1, items: [
            { text: `Factura ${data.numero}`, bold: true },
            { text: `Cliente: ${data.cliente}` },
          ] },
          { dots: 150, align: 'right', items: [{ qr: data.qr, qr_ec: 'M' }] },
        ] },
        ...data.items.map((i) => ({
          cols: [
            { weight: 3, pad: 4, border: true, items: [{ text: i.nombre, mono: true }] },
            { dots: 70, pad: 4, border: true, align: 'center' as const, items: [{ text: i.cantidad, mono: true }] },
            { dots: 130, pad: 4, border: true, align: 'right' as const, items: [{ text: i.precio, mono: true }] },
          ],
        })),
        { cols: [
          { weight: 1, items: [] },
          { dots: 300, pad: 6, items: [
            { text: `TOTAL  ${data.total}`, size: 'l', bold: true, align: 'center', invert: true },
          ] },
        ] },
      ],
    };
    return this.printLayout(station, layout);
  }

  async logs(limit = 50) {
    await this.requireReady();
    return this.client.logs(limit);
  }
}
