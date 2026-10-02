import { Injectable } from '@angular/core';
import {
  CollaTech,
  ConnectionError,
  PrinterAlias,
  PrinterInfo,
  PrintInvoiceOptions,
  PrintJob,
  PrintResult,
} from 'collatech-sdk';

@Injectable({ providedIn: 'root' })
export class CollaTechPrintService {
  private readonly client = new CollaTech({
    baseUrl: (window as any).__COLLATECH_URL__ || 'http://localhost:18743',
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

  async logs(limit = 50) {
    await this.requireReady();
    return this.client.logs(limit);
  }
}
