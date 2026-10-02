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

export interface PrintInvoiceOptions extends DocumentOptions {
  empresa?: string;
  cliente?: string;
  items?: TemplateItem[];
  total?: string;
  mensaje?: string;
}

// ---------------------------------------------------------------------------
// Print – Text
// ---------------------------------------------------------------------------

export interface PrintTextOptions extends DocumentOptions {
  text: string;
}

// ---------------------------------------------------------------------------
// Print – Ticket
// ---------------------------------------------------------------------------

export type TextAlign = "left" | "center" | "right";

/** Modo de corte. Tambien se acepta `true` (parcial) y `false` (ninguno). */
export type CutMode = "partial" | "full" | "none";

/** Ancho del papel en PUNTOS: 384 = 58 mm, 512 = 72 mm, 576 = 80 mm. */
export type PaperWidth = 384 | 512 | 576;

/** Fuente interna: "a" normal, "b" condensada (entra mas texto por linea). */
export type PrinterFont = "a" | "b";

/**
 * Opciones que valen para cualquier trabajo de impresion. Si la impresora
 * esta dada de alta, lo que se omita sale de su perfil.
 */
export interface DocumentOptions {
  printer: string;
  width?: PaperWidth;
  cut?: CutMode | boolean;
  /** Aprieta el interlineado: alrededor de un 20% menos de papel. */
  compact?: boolean;
  /** Alto de linea exacto en puntos. 0 deja el de la impresora. */
  line_spacing?: number;
  /** Imprime el ticket girado 180 grados. */
  upside_down?: boolean;
  /** Lineas en blanco antes del contenido. 0 por defecto. */
  feed_top?: number;
  /** Lineas que se avanzan al cortar. Minimo 4. */
  feed_bottom?: number;
  /** Margen izquierdo en puntos. */
  margin_dots?: number;
  font?: PrinterFont;
  /** Abre el cajon de dinero. */
  drawer?: boolean;
}

/** Una impresora dada de alta, con sus ajustes propios. */
export interface PrinterProfile {
  /** Nombre con el que se la llama al imprimir. */
  name: string;
  /** Uno o varios destinos separados por coma; con varios sale una copia en cada uno. */
  target: string;
  paper_width?: PaperWidth;
  cut?: CutMode;
  upside_down?: boolean;
  line_spacing?: number;
  font?: PrinterFont;
  feed_bottom?: number;
  description?: string;
}

/** Niveles de correccion de errores de un QR. */
export type QRErrorCorrection = "L" | "M" | "Q" | "H";

export interface QRSpec {
  data: string;
  /** Lado de cada punto, 1 a 16. Si se omite se calcula segun el papel. */
  size?: number;
  ec?: QRErrorCorrection;
}

export type BarcodeKind =
  | "code128" | "ean13" | "ean8" | "upca" | "upce"
  | "code39" | "code93" | "itf" | "codabar" | "pdf417";

export interface BarcodeSpec {
  data: string;
  type?: BarcodeKind;
  /** Alto en puntos, 1 a 255. */
  height?: number;
  /** Grosor de la barra fina, 2 a 6. Si se omite se calcula segun el papel. */
  width?: number;
  /** Donde va el texto legible. */
  hri?: "none" | "above" | "below" | "both";
}

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

/**
 * Un elemento del ticket. El campo `type` decide cual de los demas se usa,
 * de forma que el ticket se describe de arriba abajo en un solo array.
 */
export interface TicketLine {
  type?: "text" | "table" | "qr" | "barcode" | "image" | "rule" | "feed";

  text?: string;
  table?: TicketTable;
  qr?: QRSpec;
  barcode?: BarcodeSpec;
  /** Imagen en base64 o data URI. */
  image?: string;
  /** Caracter con el que se dibuja una linea separadora. */
  rule?: string;
  /** Lineas en blanco de un elemento de tipo feed. */
  feed?: number;

  align?: TextAlign;
  bold?: boolean;
  underline?: boolean;
  /** Blanco sobre negro. */
  invert?: boolean;
  size?: FontSize;
  /** Multiplicador exacto, 1 a 8. Manda sobre `size`. */
  scale_w?: number;
  scale_h?: number;

  /** Lineas en blanco despues de esta. */
  gap?: number;
  /** Enmarca el texto en un recuadro. */
  box?: boolean;
  /** Margen izquierdo, en caracteres. */
  ml?: number;
  /** Margen derecho, en caracteres. */
  mr?: number;
}

export interface PrintTicketOptions extends DocumentOptions {
  title?: string;
  lines?: TicketLine[];
  /** PNG/JPEG/GIF en base64 o data URI. */
  logo?: string;
  /** Escala de la imagen en porcentaje, 35-100. */
  scale?: number;
  /** Atajo: equivale a un elemento qr al final. Hasta 2953 caracteres. */
  qr?: string;
  /** Atajo: equivale a un elemento barcode al final. Hasta 253 caracteres. */
  barcode?: string;
  /** Dibuja un marco alrededor de todo el ticket. */
  border?: boolean;
  margin_left?: number;
  margin_right?: number;
}

// ---------------------------------------------------------------------------
// Print – Template
// ---------------------------------------------------------------------------

export interface PrintTemplateOptions extends DocumentOptions {
  template: TemplateName;
  data?: TemplateData;
}

// ---------------------------------------------------------------------------
// Print – HTML
// ---------------------------------------------------------------------------

export interface PrintHTMLOptions extends DocumentOptions {
  html: string;
}

// ---------------------------------------------------------------------------
// Print – Image
// ---------------------------------------------------------------------------

export interface PrintImageOptions extends DocumentOptions {
  image: string;
  /** Escala en porcentaje, 35-100. */
  scale?: number;
}

// ---------------------------------------------------------------------------
// Print – Logo
// ---------------------------------------------------------------------------

export interface PrintLogoOptions extends DocumentOptions {
  /** Escala en porcentaje, 35-100. */
  scale?: number;
}

// ---------------------------------------------------------------------------
// Print – Raw
// ---------------------------------------------------------------------------

export interface PrintRawOptions extends DocumentOptions {
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
