// ============================================================
// COLLATECH - ARCHIVO COMPLETO v2
// Copia en: src/app/collatech.ts
// Ruta del agente: http://localhost:18743
// ============================================================

const URL = 'http://127.0.0.1:18743';

// ============================================================
// FUNCION INTERNA - No tocar
// ============================================================
async function api(path: string, body?: any): Promise<any> {
  const options: RequestInit = {
    method: body ? 'POST' : 'GET',
    headers: {
      'Content-Type': 'application/json',
      'Accept': 'application/json',
    },
  };
  if (body) {
    options.body = JSON.stringify(body);
  }

  console.log(`[CollaTech] ${options.method} ${URL}${path}`);
  const res = await fetch(`${URL}${path}`, options);
  const json = await res.json();

  console.log(`[CollaTech] Response:`, json);

  if (!json.ok) {
    throw new Error(json.error || 'Error desconocido');
  }

  return json.data;
}

// ============================================================
// HEALTH - Verificar conexion
// ============================================================
export async function ctHealth(): Promise<any> {
  return api('/health');
}

// ============================================================
// PRINTERS - Listar impresoras
// ============================================================
export async function ctPrinters(): Promise<any[]> {
  return api('/api/printers');
}

// ============================================================
// STATUS - Estado del agente
// ============================================================
export async function ctStatus(): Promise<any> {
  return api('/api/status');
}

// ============================================================
// NETWORK - Rutas LAN del agente
// ============================================================
export async function ctNetwork(): Promise<any> {
  return api('/api/network');
}

// ============================================================
// READY - Verificar si el agente esta listo
// ============================================================
export async function ctIsReady(): Promise<boolean> {
  try {
    await ctHealth();
    return true;
  } catch {
    return false;
  }
}

export async function ctWaitUntilReady(retries = 20, intervalMs = 500): Promise<boolean> {
  for (let i = 0; i < retries; i++) {
    if (await ctIsReady()) return true;
    await new Promise((resolve) => setTimeout(resolve, intervalMs));
  }
  return false;
}

// ============================================================
// TEMPLATES - Listar templates
// ============================================================
export async function ctTemplates(): Promise<string[]> {
  return api('/api/templates');
}

// ============================================================
// SETTINGS - Obtener configuracion
// ============================================================
export async function ctGetSettings(): Promise<any> {
  return api('/api/settings');
}

// ============================================================
// SETTINGS - Guardar configuracion
// ============================================================
export async function ctSaveSettings(settings: {
  default_printer?: string;
  paper_width?: number;
  image_scale?: number;
}): Promise<any> {
  return api('/api/settings', settings);
}

// ============================================================
// LOGS - Obtener logs
// ============================================================
export async function ctLogs(limit = 80): Promise<any[]> {
  return api(`/api/logs?limit=${limit}`);
}

// ============================================================
// ESTACIONES / ALIAS - cocina, recepcion, facturas, pagos
// ============================================================
export interface PrinterAlias {
  name: string;
  printer: string;       // Puede ser una o varias: "EPSON Cocina,EPSON Barra"
  description?: string;
}

export async function ctPrinterAliases(): Promise<PrinterAlias[]> {
  return api('/api/printer-aliases');
}

export async function ctSavePrinterAliases(aliases: PrinterAlias[]): Promise<PrinterAlias[]> {
  return api('/api/printer-aliases', aliases);
}

export async function ctConfigureDefaultStations(realPrinter: string): Promise<PrinterAlias[]> {
  return ctSavePrinterAliases([
    { name: 'cocina', printer: realPrinter, description: 'Pedidos de cocina' },
    { name: 'recepcion', printer: realPrinter, description: 'Recepcion y caja' },
    { name: 'facturas', printer: realPrinter, description: 'Facturas' },
    { name: 'pagos', printer: realPrinter, description: 'Pagos y recibos' },
  ]);
}

// ============================================================
// PRINT TEXT - Imprimir texto plano
// ============================================================
export async function ctPrintText(
  printer: string,
  text: string,
  cut = true
): Promise<any> {
  return api('/api/print/text', { printer, text, cut });
}

export async function ctPrintToStation(
  station: string,
  text: string,
  cut = true
): Promise<any> {
  return ctPrintText(station, text, cut);
}

export async function ctTestStation(station: string, copies = 1): Promise<any[]> {
  const total = Math.max(1, Math.min(5, Math.floor(copies)));
  const results: any[] = [];
  for (let i = 1; i <= total; i++) {
    results.push(await ctPrintText(
      station,
      `COLLATECH AGENT\nPrueba estacion: ${station}\nCopia: ${i}/${total}\n${new Date().toLocaleString()}`,
      true
    ));
  }
  return results;
}

// ============================================================
// PRINT TICKET - Imprimir ticket estructurado
// ============================================================
export interface TableColumn {
  text: string;
  width: number;
  align?: 'left' | 'center' | 'right';
}

export interface TableDef {
  border?: boolean;
  header?: boolean;
  columns: TableColumn[];
  rows: string[][];
}

export interface TicketLine {
  type?: string;          // "text" (default) o "table"
  text?: string;
  table?: TableDef;
  align?: 'left' | 'center' | 'right';
  bold?: boolean;
  underline?: boolean;
  size?: 'normal' | 'double' | 'small' | 'wide' | 'tall';
  gap?: number;
  box?: boolean;
  ml?: number;
  mr?: number;
}

export async function ctPrintTicket(data: {
  printer: string;
  title?: string;
  lines?: TicketLine[];
  qr?: string;
  barcode?: string;
  logo?: string;
  cut?: boolean;
  drawer?: boolean;
  feed_top?: number;
  feed_bottom?: number;
  border?: boolean;
  margin_left?: number;
  margin_right?: number;
}): Promise<any> {
  return api('/api/print/ticket', { cut: true, ...data });
}

export async function ctOpenDrawer(printer: string): Promise<any> {
  return ctPrintTicket({
    printer,
    drawer: true,
    cut: false,
    lines: [{ text: 'Apertura de caja' }],
  });
}

// ============================================================
// PRINT HTML - Imprimir HTML (ESC/POS nativo)
// ============================================================
export async function ctPrintHTML(
  printer: string,
  html: string,
  cut = true
): Promise<any> {
  return api('/api/print/html', { printer, html, cut });
}

// ============================================================
// PRINT TEMPLATE - Imprimir con template
// ============================================================
export async function ctPrintTemplate(data: {
  printer: string;
  template: string;
  data?: Record<string, any>;
  cut?: boolean;
}): Promise<any> {
  return api('/api/print/template', { cut: true, ...data });
}

export async function ctPrintInvoice(data: {
  printer: string;
  empresa?: string;
  cliente?: string;
  items?: Array<{ nombre: string; precio?: string }>;
  total?: string;
  mensaje?: string;
  cut?: boolean;
}): Promise<any> {
  return ctPrintTemplate({
    printer: data.printer,
    template: 'factura.html',
    cut: data.cut ?? true,
    data: {
      empresa: data.empresa,
      cliente: data.cliente,
      items: data.items,
      total: data.total,
      mensaje: data.mensaje,
    },
  });
}

export async function ctPrintReceipt(data: {
  printer: string;
  empresa?: string;
  cliente?: string;
  total?: string;
  mensaje?: string;
  cut?: boolean;
}): Promise<any> {
  return ctPrintTemplate({
    printer: data.printer,
    template: 'recibo.html',
    cut: data.cut ?? true,
    data: {
      empresa: data.empresa,
      cliente: data.cliente,
      total: data.total,
      mensaje: data.mensaje,
    },
  });
}

// ============================================================
// PRINT IMAGE - Imprimir imagen base64
// ============================================================
export async function ctPrintImage(
  printer: string,
  image: string,
  width = 384,
  cut = true
): Promise<any> {
  return api('/api/print/image', { printer, image, width, cut });
}

// ============================================================
// PRINT LOGO - Imprimir LOGO.png por defecto
// ============================================================
export async function ctPrintLogo(
  printer: string,
  cut = true
): Promise<any> {
  return api('/api/print/logo', { printer, cut });
}

// ============================================================
// PRINT RAW - Enviar ESC/POS raw
// ============================================================
export async function ctPrintRaw(
  printer: string,
  data: string,
  base64 = false
): Promise<any> {
  return api('/api/print/raw', { printer, data, base64 });
}

// ============================================================
// FILE TO BASE64 - Convertir archivo a base64
// ============================================================
export function ctFileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(new Error('Error leyendo archivo'));
    reader.readAsDataURL(file);
  });
}

// ============================================================
// IMPRIMIR - Funcion unificada para todo
// ============================================================
export type PrintType = 'text' | 'ticket' | 'html' | 'template' | 'image' | 'logo' | 'raw';

export interface PrintOptions {
  printer: string;
  type: PrintType;
  text?: string;
  title?: string;
  lines?: TicketLine[];
  html?: string;
  template?: string;
  templateData?: Record<string, any>;
  image?: string;
  imageWidth?: number;
  rawData?: string;
  rawBase64?: boolean;
  qr?: string;
  barcode?: string;
  logo?: string;
  cut?: boolean;
  drawer?: boolean;
  feed_top?: number;
  feed_bottom?: number;
  border?: boolean;
}

export async function ctImprimir(options: PrintOptions): Promise<any> {
  const { printer, type, cut = true, ...rest } = options;

  switch (type) {
    case 'text':
      return ctPrintText(printer, rest.text || '', cut);

    case 'ticket':
      return ctPrintTicket({
        printer,
        title: rest.title,
        lines: rest.lines,
        qr: rest.qr,
        barcode: rest.barcode,
        logo: rest.logo,
        drawer: rest.drawer,
        feed_top: rest.feed_top,
        feed_bottom: rest.feed_bottom,
        border: rest.border,
        cut,
      });

    case 'html':
      return ctPrintHTML(printer, rest.html || '', cut);

    case 'template':
      return ctPrintTemplate({
        printer,
        template: rest.template || '',
        data: rest.templateData,
        cut,
      });

    case 'image':
      return ctPrintImage(printer, rest.image || '', rest.imageWidth, cut);

    case 'logo':
      return ctPrintLogo(printer, cut);

    case 'raw':
      return ctPrintRaw(printer, rest.rawData || '', rest.rawBase64);

    default:
      throw new Error(`Tipo desconocido: ${type}`);
  }
}

// ============================================================
// TICKET LINE - Funcion auxiliar para crear lines
// ============================================================
export function ctLine(
  text: string,
  opts?: {
    align?: 'left' | 'center' | 'right';
    bold?: boolean;
    underline?: boolean;
    size?: 'normal' | 'double' | 'small' | 'wide' | 'tall';
    gap?: number;
    box?: boolean;
    ml?: number;
    mr?: number;
  }
): TicketLine {
  return { text, ...opts };
}

export function ctTable(tbl: TableDef): TicketLine {
  return { type: 'table', table: tbl };
}
