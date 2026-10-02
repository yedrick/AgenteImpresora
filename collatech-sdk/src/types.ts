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

export interface HealthData {
  ok: boolean;
  message: string;
}

export interface StatusData {
  service: string;
  time: string;
  jobs: PrintJob[];
}

export interface PrintJob {
  id: string;
  printer: string;
  status: "Pending" | "Printing" | "Completed" | "Failed" | string;
  state?: string;
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
  type: "windows" | "tcp" | "file";
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

export interface TicketLine {
  text: string;
  align?: TextAlign;
  bold?: boolean;
  underline?: boolean;
}

export interface PrintTicketOptions {
  printer: string;
  title?: string;
  lines?: TicketLine[];
  qr?: string;
  barcode?: string;
  logo?: string;
  width?: number;
  scale?: number;
  cut?: boolean;
  drawer?: boolean;
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

export interface LogEntry {
  level?: string;
  message?: string;
  time?: string;
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
  /** Custom fetch implementation (for Node.js < 18 or edge runtimes). */
  fetch?: typeof globalThis.fetch;
}
