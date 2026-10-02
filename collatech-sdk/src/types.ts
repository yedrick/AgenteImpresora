// ---------------------------------------------------------------------------
// CollaTech Agent SDK – Type Definitions
// ---------------------------------------------------------------------------

/** Standard API response envelope. */
export interface CollaTechResponse<T = unknown> {
  ok: boolean;
  message?: string;
  data?: T;
  error?: string;
}

// ---------------------------------------------------------------------------
// Health / Status
// ---------------------------------------------------------------------------

/**
 * El agente responde a /health con `{ok, message}` y SIN campo `data`, y el
 * cliente desenvuelve `data`. Por eso `health()` resuelve a `undefined`: lo
 * que importa es que no lance. Usa `isReady()` si solo quieres un booleano.
 */
export type HealthData = void;

export interface StatusData {
  service: string;
  time: string;
  jobs: PrintJob[];
}

export interface PrintJob {
  id: string;
  printer: string;
  status: "Pending" | "Printing" | "Completed" | "Failed" | string;
  attempts?: number;
  error?: string;
  created_at: string;
  updated_at?: string;
}

export type PrintResult = PrintJob | PrintJob[];

export interface WaitUntilReadyOptions {
  retries?: number;
  intervalMs?: number;
}

export interface NetworkInfo {
  hostname?: string;
  host: string;
  port: number;
  allow_remote: boolean;
  urls: string[];
}

// ---------------------------------------------------------------------------
// Printers
// ---------------------------------------------------------------------------

export interface PrinterInfo {
  name: string;
  /** Detalle del estado reportado por Windows ("lista", "sin papel", ...). */
  status?: string;
  type: "windows" | "tcp" | "com" | "file" | string;
  address: string;
  online: boolean;
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

export interface Settings {
  default_printer?: string;
  paper_width?: number;
  image_scale?: number;
  aliases?: PrinterAlias[];
}

export interface PrinterAlias {
  name: string;
  printer: string;
  description?: string;
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

export type TemplateName = "recibo" | "comanda" | "texto" | "qr" | "imagen" | string;

export interface TemplateItem {
  nombre: string;
  precio?: string;
}

export interface TemplateData {
  empresa?: string;
  cliente?: string;
  total?: string;
  mensaje?: string;
  qr?: string;
  barcode?: string;
  items?: TemplateItem[];
}

export interface PrintInvoiceOptions {
  printer: string;
  empresa?: string;
  cliente?: string;
  items?: TemplateItem[];
  total?: string;
  mensaje?: string;
  width?: number;
  cut?: boolean;
}

// ---------------------------------------------------------------------------
// Print – Text
// ---------------------------------------------------------------------------

export interface PrintTextOptions {
  printer: string;
  text: string;
  cut?: boolean;
}

// ---------------------------------------------------------------------------
// Print – Ticket
// ---------------------------------------------------------------------------

export type TextAlign = "left" | "center" | "right";

/** Tamanos de fuente que entiende el agente. */
export type FontSize = "normal" | "double" | "wide" | "tall" | "small";

export interface TableColumn {
  text: string;
  width: number;
  align?: TextAlign;
}

export interface TicketTable {
  columns: TableColumn[];
  /** Cada fila debe tener como mucho tantas celdas como columnas. */
  rows: string[][];
  header?: boolean;
  border?: boolean;
}

export interface TicketLine {
  /** "table" dibuja `table`; vacio o ausente imprime `text`. */
  type?: "text" | "table";
  text?: string;
  table?: TicketTable;
  align?: TextAlign;
  bold?: boolean;
  underline?: boolean;
  size?: FontSize;
  /** Lineas en blanco despues de esta. */
  gap?: number;
  /** Enmarca el texto en un recuadro. */
  box?: boolean;
  /** Margen izquierdo, en caracteres. */
  ml?: number;
  /** Margen derecho, en caracteres. */
  mr?: number;
}

export interface PrintTicketOptions {
  printer: string;
  title?: string;
  lines?: TicketLine[];
  /** Hasta 2953 caracteres. */
  qr?: string;
  /** Hasta 253 caracteres (Code128). */
  barcode?: string;
  /** PNG/JPEG/GIF en base64 o data URI. */
  logo?: string;
  /** Ancho del papel en PUNTOS: 384 (58mm), 512 (72mm) o 576 (80mm). */
  width?: number;
  /** Escala de la imagen en porcentaje, 35-100. */
  scale?: number;
  cut?: boolean;
  drawer?: boolean;
  /** Lineas en blanco al principio. */
  feed_top?: number;
  /** Lineas en blanco antes del corte (minimo 4). */
  feed_bottom?: number;
  /** Dibuja un marco alrededor de todo el ticket. */
  border?: boolean;
  margin_left?: number;
  margin_right?: number;
}

// ---------------------------------------------------------------------------
// Print – Template
// ---------------------------------------------------------------------------

export interface PrintTemplateOptions {
  printer: string;
  template: TemplateName;
  data?: TemplateData;
  width?: number;
  cut?: boolean;
}

// ---------------------------------------------------------------------------
// Print – HTML
// ---------------------------------------------------------------------------

export interface PrintHTMLOptions {
  printer: string;
  html: string;
  width?: number;
  cut?: boolean;
}

// ---------------------------------------------------------------------------
// Print – Image
// ---------------------------------------------------------------------------

export interface PrintImageOptions {
  printer: string;
  image: string;
  width?: number;
  scale?: number;
  cut?: boolean;
}

// ---------------------------------------------------------------------------
// Print – Logo
// ---------------------------------------------------------------------------

export interface PrintLogoOptions {
  printer: string;
  width?: number;
  scale?: number;
  cut?: boolean;
}

// ---------------------------------------------------------------------------
// Print – Raw
// ---------------------------------------------------------------------------

export interface PrintRawOptions {
  printer: string;
  data: string;
  base64?: boolean;
}

// ---------------------------------------------------------------------------
// Logs
// ---------------------------------------------------------------------------

/** Una linea de logs/YYYY-MM-DD.jsonl. */
export interface LogEntry {
  time?: string;
  level?: "info" | "error" | string;
  /** Nombre del evento: "print_enqueued", "print_status", ... */
  event?: string;
  details?: Record<string, unknown>;
  [key: string]: unknown;
}

// ---------------------------------------------------------------------------
// SDK Configuration
// ---------------------------------------------------------------------------

export interface CollaTechConfig {
  /** Base URL of the CollaTech Agent. Defaults to http://localhost:18743 */
  baseUrl?: string;
  /** Request timeout in milliseconds. Defaults to 10000 */
  timeout?: number;
  /** Enable console debug logs from the SDK. Defaults to false. */
  debug?: boolean;
  /**
   * Token de acceso. El agente lo exige en /api/* a las peticiones que no
   * vienen de su propia PC. Lo muestra el panel local y el instalador al
   * terminar. No hace falta si el codigo corre en la misma PC que el agente.
   */
  token?: string;
  /** Custom fetch implementation (for Node.js < 18 or edge runtimes). */
  fetch?: typeof globalThis.fetch;
}
