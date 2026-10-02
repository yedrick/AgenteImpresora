// ---------------------------------------------------------------------------
// CollaTech Agent SDK – Client
// ---------------------------------------------------------------------------

import type {
  CollaTechConfig,
  CollaTechResponse,
  HealthData,
  StatusData,
  NetworkInfo,
  PrinterInfo,
  PrinterAlias,
  Settings,
  PrintResult,
  WaitUntilReadyOptions,
  PrintInvoiceOptions,
  PrintTextOptions,
  PrintTicketOptions,
  PrintTemplateOptions,
  PrintHTMLOptions,
  PrintImageOptions,
  PrintLogoOptions,
  PrintRawOptions,
  LogEntry,
  PaperWidth,
  PrinterProfile,
  CutMode,
  PrinterFont,
  QRSpec,
  BarcodeSpec,
  DocumentOptions,
  TicketLine,
  TicketTable,
  TextAlign,
  FontSize,
} from "./types.js";

import {
  CollaTechError,
  ConnectionError,
  ApiError,
  ForbiddenError,
  UnauthorizedError,
  QueueFullError,
  ValidationError,
} from "./errors.js";

import { normalizeUrl } from "./utils.js";

const DEFAULT_BASE_URL = "http://localhost:18743";
const DEFAULT_TIMEOUT = 10_000;

/** Limites del agente (internal/escpos/builder.go). */
export const MAX_QR_LEN = 2953;
export const MAX_BARCODE_LEN = 253;

/**
 * El agente exige un token a las peticiones que no vienen de su propia PC.
 * Ademas de pasarlo en la configuracion, se admite `window.__COLLATECH_TOKEN__`
 * para inyectarlo desde el HTML sin recompilar.
 */
function resolveToken(): string {
  if (typeof window !== "undefined") {
    const w = window as any;
    if (w.__COLLATECH_TOKEN__) return w.__COLLATECH_TOKEN__;
    if (w.collatech?.token) return w.collatech.token;
  }
  return "";
}

function resolveBaseUrl(): string {
  if (typeof window !== "undefined") {
    const w = window as any;
    if (w.__COLLATECH_URL__) return w.__COLLATECH_URL__;
    if (w.collatech?.baseUrl) return w.collatech.baseUrl;
  }
  return DEFAULT_BASE_URL;
}

/**
 * CollaTech Agent SDK client.
 *
 * Works in any JavaScript environment: browser (React, Angular, Vue, Next.js),
 * Node.js (≥ 18), Deno, Bun, and edge runtimes.
 *
 * @example
 * ```ts
 * import { CollaTech } from "collatech-sdk";
 *
 * const printer = new CollaTech();
 *
 * // Health check
 * await printer.health();
 *
 * // Print text
 * await printer.printText({ printer: "Impresora1", text: "Hello!" });
 * ```
 */
export class CollaTech {
  private readonly baseUrl: string;
  private readonly timeout: number;
  private readonly debug: boolean;
  private readonly token: string;
  private readonly _fetch: typeof globalThis.fetch;

  constructor(config?: CollaTechConfig) {
    this.baseUrl = normalizeUrl(config?.baseUrl ?? resolveBaseUrl());
    this.timeout = config?.timeout ?? DEFAULT_TIMEOUT;
    this.debug = config?.debug ?? false;
    this.token = config?.token ?? resolveToken();
    this._fetch = config?.fetch ?? globalThis.fetch;
    this.log(`[CollaTech] SDK initialized. URL: ${this.baseUrl}`);
  }

  // -------------------------------------------------------------------------
  // Health / Status
  // -------------------------------------------------------------------------

  /** Check if the agent is running and healthy. */
  async health(): Promise<HealthData> {
    const res = await this.get<HealthData>("/health");
    return res;
  }

  /** Return true when the agent is reachable. */
  async isReady(): Promise<boolean> {
    try {
      await this.health();
      return true;
    } catch {
      return false;
    }
  }

  /** Wait until the local agent is reachable. Useful before first print. */
  async waitUntilReady(options: WaitUntilReadyOptions = {}): Promise<boolean> {
    const retries = options.retries ?? 20;
    const intervalMs = options.intervalMs ?? 500;
    for (let i = 0; i < retries; i++) {
      if (await this.isReady()) return true;
      await delay(intervalMs);
    }
    return false;
  }

  /** Impresoras dadas de alta, con su papel, corte y giro. */
  async printerProfiles(): Promise<PrinterProfile[]> {
    return (await this.get<PrinterProfile[]>("/api/printers-config")) ?? [];
  }

  /**
   * Reemplaza la lista de impresoras dadas de alta. Solo responde desde la
   * propia maquina del agente.
   */
  async savePrinterProfiles(list: PrinterProfile[]): Promise<PrinterProfile[]> {
    return (await this.post<PrinterProfile[]>("/api/printers-config", list)) ?? [];
  }

  /**
   * Descarga el paquete de soporte: un zip con el diagnostico, la
   * configuracion, las impresoras, la cola y el registro reciente, con el
   * token tapado. Solo responde desde la propia maquina del agente.
   */
  async supportBundle(): Promise<Blob> {
    const res = await this._fetch(`${this.baseUrl}/api/support-bundle`, {
      method: "GET",
      headers: this.headers(),
    });
    if (!res.ok) {
      throw new ApiError(res.status, "no se pudo generar el paquete de soporte");
    }
    return res.blob();
  }

  /** Get agent status including current time and queued print jobs. */
  async status(): Promise<StatusData> {
    const res = await this.get<StatusData>("/api/status");
    return res;
  }

  /** Get LAN URLs and host information for this agent. */
  async network(): Promise<NetworkInfo> {
    const res = await this.get<NetworkInfo>("/api/network");
    return res;
  }

  // -------------------------------------------------------------------------
  // Printers
  // -------------------------------------------------------------------------

  /** List all detected printers (Windows + COM ports). */
  async printers(): Promise<PrinterInfo[]> {
    const res = await this.get<PrinterInfo[]>("/api/printers");
    return res;
  }

  // -------------------------------------------------------------------------
  // Templates
  // -------------------------------------------------------------------------

  /** List available built-in template names. */
  async templates(): Promise<string[]> {
    const res = await this.get<string[]>("/api/templates");
    return res;
  }

  // -------------------------------------------------------------------------
  // Settings
  // -------------------------------------------------------------------------

  /** Load current user settings. */
  async getSettings(): Promise<Settings> {
    const res = await this.get<Settings>("/api/settings");
    return res;
  }

  /** Save user settings. */
  async saveSettings(settings: Settings): Promise<Settings> {
    const res = await this.post<Settings>("/api/settings", settings);
    return res;
  }

  /** List configured logical printer aliases, like cocina, recepcion, facturas. */
  async printerAliases(): Promise<PrinterAlias[]> {
    const res = await this.get<PrinterAlias[]>("/api/printer-aliases");
    return res;
  }

  /** Save logical printer aliases used by print calls. */
  async savePrinterAliases(aliases: PrinterAlias[]): Promise<PrinterAlias[]> {
    const res = await this.post<PrinterAlias[]>("/api/printer-aliases", aliases);
    return res;
  }

  /** Save common POS stations in one call. */
  async configurePrinterAliases(aliases: PrinterAlias[]): Promise<PrinterAlias[]> {
    return this.savePrinterAliases(aliases);
  }

  // -------------------------------------------------------------------------
  // Logs
  // -------------------------------------------------------------------------

  /** Get the last `limit` log entries (max 500). */
  async logs(limit = 80): Promise<LogEntry[]> {
    const res = await this.get<LogEntry[]>(`/api/logs?limit=${limit}`);
    return res;
  }

  // -------------------------------------------------------------------------
  // Print – Text
  // -------------------------------------------------------------------------

  /**
   * Print plain text.
   *
   * @example
   * ```ts
   * await printer.printText({
   *   printer: "Impresora1",
   *   text: "Hola Mundo",
   *   cut: true,
   * });
   * ```
   */
  async printText(options: PrintTextOptions): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/text", {
      printer: options.printer,
      text: options.text,
      cut: options.cut ?? true,
    });
  }

  /** Print text to a station alias, for example cocina, recepcion, facturas. */
  async printToStation(
    station: string,
    text: string,
    cut = true
  ): Promise<PrintResult> {
    return this.printText({ printer: station, text, cut });
  }

  /** Send a quick diagnostic ticket to one station or a comma-separated printer list. */
  async testStation(station: string, copies = 1): Promise<PrintResult[]> {
    const total = Math.max(1, Math.min(5, Math.floor(copies)));
    const results: PrintResult[] = [];
    for (let i = 1; i <= total; i++) {
      results.push(
        await this.printText({
          printer: station,
          text: `COLLATECH AGENT\nPrueba estacion: ${station}\nCopia: ${i}/${total}\n${new Date().toLocaleString()}`,
          cut: true,
        })
      );
    }
    return results;
  }

  // -------------------------------------------------------------------------
  // Print – Ticket
  // -------------------------------------------------------------------------

  /**
   * Print a structured ticket with title, lines, QR, barcode, and logo.
   *
   * @example
   * ```ts
   * await printer.printTicket({
   *   printer: "Impresora1",
   *   title: "Mi Tienda",
   *   lines: [
   *     { text: "Producto A", align: "left" },
   *     { text: "Bs 100.00", align: "right", bold: true },
   *   ],
   *   qr: "https://example.com",
   *   cut: true,
   * });
   * ```
   */
  async printTicket(options: PrintTicketOptions): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/ticket", {
      printer: options.printer,
      title: options.title,
      lines: options.lines,
      qr: options.qr,
      barcode: options.barcode,
      logo: options.logo,
      width: options.width,
      scale: options.scale,
      cut: options.cut ?? true,
      drawer: options.drawer ?? false,
    });
  }

  /** Open a cash drawer connected to a printer. */
  async openDrawer(printer: string): Promise<PrintResult> {
    return this.printTicket({
      printer,
      drawer: true,
      lines: [{ text: "Apertura de caja" }],
      cut: false,
    });
  }

  // -------------------------------------------------------------------------
  // Print – Template
  // -------------------------------------------------------------------------

  /**
   * Print using a built-in template (recibo, comanda, texto, qr, imagen,
   * or default factura).
   *
   * @example
   * ```ts
   * await printer.printTemplate({
   *   printer: "Impresora1",
   *   template: "recibo",
   *   data: {
   *     empresa: "Mi Empresa",
   *     cliente: "Juan Perez",
   *     total: "Bs 250.00",
   *     items: [
   *       { nombre: "Cafe", precio: "Bs 5.00" },
   *       { nombre: "Te", precio: "Bs 3.00" },
   *     ],
   *   },
   * });
   * ```
   */
  async printTemplate(
    options: PrintTemplateOptions
  ): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/template", {
      printer: options.printer,
      template: options.template,
      data: options.data,
      width: options.width,
      cut: options.cut ?? true,
    });
  }

  /** Print the built-in factura template with a simple typed payload. */
  async printInvoice(options: PrintInvoiceOptions): Promise<PrintResult> {
    return this.printTemplate({
      printer: options.printer,
      template: "factura.html",
      width: options.width ?? 576,
      cut: options.cut ?? true,
      data: {
        empresa: options.empresa,
        cliente: options.cliente,
        items: options.items,
        total: options.total,
        mensaje: options.mensaje,
      },
    });
  }

  /** Print the built-in recibo template with a simple typed payload. */
  async printReceipt(options: PrintInvoiceOptions): Promise<PrintResult> {
    return this.printTemplate({
      printer: options.printer,
      template: "recibo.html",
      width: options.width ?? 576,
      cut: options.cut ?? true,
      data: {
        empresa: options.empresa,
        cliente: options.cliente,
        items: options.items,
        total: options.total,
        mensaje: options.mensaje,
      },
    });
  }

  // -------------------------------------------------------------------------
  // Print – HTML
  // -------------------------------------------------------------------------

  /**
   * Render HTML directly to ESC/POS native commands (fast, no image).
   * Supports: bold, underline, center, tables, hr, headings.
   * Use this for fully custom invoice / receipt designs.
   *
   * @example
   * ```ts
   * await printer.printHTML({
   *   printer: "Impresora1",
   *   html: `
   *     <center><b>MI EMPRESA</b></center>
   *     <hr>
   *     <p>Factura #00123</p>
   *     <table>
   *       <tr><td>Cafe</td><td style="text-align:right">Bs 5.00</td></tr>
   *     </table>
   *     <hr>
   *     <p style="text-align:right"><b>Total: Bs 150.00</b></p>
   *   `,
   *   cut: true,
   * });
   * ```
   */
  async printHTML(options: PrintHTMLOptions): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/html", {
      printer: options.printer,
      html: options.html,
      width: options.width,
      cut: options.cut ?? true,
    });
  }

  // -------------------------------------------------------------------------
  // Print – Image
  // -------------------------------------------------------------------------

  /**
   * Print a base64-encoded image (PNG, JPEG, GIF).
   * Use {@link fileToBase64} to convert a File/Blob from the browser.
   *
   * @example
   * ```ts
   * const { fileToBase64 } = await import("collatech-sdk");
   * const base64 = await fileToBase64(fileInput.files[0]);
   * await printer.printImage({
   *   printer: "Impresora1",
   *   image: base64,
   *   cut: true,
   * });
   * ```
   */
  async printImage(options: PrintImageOptions): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/image", {
      printer: options.printer,
      image: options.image,
      width: options.width,
      scale: options.scale,
      cut: options.cut ?? true,
    });
  }

  // -------------------------------------------------------------------------
  // Print – Logo
  // -------------------------------------------------------------------------

  /**
   * Print the default LOGO.png file from the agent's directory.
   */
  async printLogo(options: PrintLogoOptions): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/logo", {
      printer: options.printer,
      width: options.width,
      scale: options.scale,
      cut: options.cut ?? true,
    });
  }

  // -------------------------------------------------------------------------
  // Print – Raw ESC/POS
  // -------------------------------------------------------------------------

  /**
   * Send raw ESC/POS bytes directly to the printer.
   *
   * @example
   * ```ts
   * await printer.printRaw({
   *   printer: "Impresora1",
   *   data: "GUFjQUFBSUFBQUFB", // base64-encoded ESC/POS
   *   base64: true,
   * });
   * ```
   */
  async printRaw(options: PrintRawOptions): Promise<PrintResult> {
    return this.post<PrintResult>("/api/print/raw", {
      printer: options.printer,
      data: options.data,
      base64: options.base64 ?? false,
    });
  }

  // -------------------------------------------------------------------------
  // Fluent Helpers – Builder-style printing
  // -------------------------------------------------------------------------

  /**
   * Create a builder for constructing and sending print jobs fluently.
   *
   * @example
   * ```ts
   * await printer
   *   .createJob("Impresora1")
   *   .title("Mi Tienda")
   *   .line("Producto A", { align: "left" })
   *   .line("Bs 100.00", { align: "right", bold: true })
   *   .qr("https://example.com")
   *   .print();
   * ```
   */
  createJob(printer: string): PrintJobBuilder {
    return new PrintJobBuilder(this, printer);
  }

  // -------------------------------------------------------------------------
  // Internal HTTP helpers
  // -------------------------------------------------------------------------

  /** Cabeceras comunes, con el token si hay uno configurado. */
  private headers(extra?: Record<string, string>): Record<string, string> {
    const headers: Record<string, string> = { Accept: "application/json", ...extra };
    if (this.token) headers["Authorization"] = `Bearer ${this.token}`;
    return headers;
  }

  private async get<T>(path: string): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    this.log(`[CollaTech] GET ${url}`);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const res = await this._fetch(url, {
        method: "GET",
        headers: this.headers(),
        signal: controller.signal,
      });

      clearTimeout(timer);
      this.log(`[CollaTech] GET ${url} -> ${res.status}`);
      return this.handleResponse<T>(res);
    } catch (err: unknown) {
      clearTimeout(timer);
      this.error(`[CollaTech] GET ${url} ERROR:`, err);
      if (err instanceof CollaTechError) throw err;
      if (isAbortError(err)) {
        throw new ConnectionError(
          `Request to ${url} timed out after ${this.timeout}ms`
        );
      }
      throw new ConnectionError(
        `Cannot connect to CollaTech Agent at ${this.baseUrl}. Is it running?`
      );
    }
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    const url = `${this.baseUrl}${path}`;
    this.log(`[CollaTech] POST ${url}`, body);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeout);

    try {
      const res = await this._fetch(url, {
        method: "POST",
        headers: this.headers({ "Content-Type": "application/json" }),
        body: JSON.stringify(body),
        signal: controller.signal,
      });

      clearTimeout(timer);
      this.log(`[CollaTech] POST ${url} -> ${res.status}`);
      return this.handleResponse<T>(res);
    } catch (err: unknown) {
      clearTimeout(timer);
      this.error(`[CollaTech] POST ${url} ERROR:`, err);
      if (err instanceof CollaTechError) throw err;
      if (isAbortError(err)) {
        throw new ConnectionError(
          `Request to ${url} timed out after ${this.timeout}ms`
        );
      }
      throw new ConnectionError(
        `Cannot connect to CollaTech Agent at ${this.baseUrl}. Is it running?`
      );
    }
  }

  private async handleResponse<T>(res: Response): Promise<T> {
    if (res.status === 401) {
      this.error(`[CollaTech] Response 401 Unauthorized`);
      throw new UnauthorizedError();
    }

    if (res.status === 403) {
      this.error(`[CollaTech] Response 403 Forbidden`);
      throw new ForbiddenError();
    }

    if (res.status === 503) {
      this.error(`[CollaTech] Response 503 Queue full`);
      throw new QueueFullError();
    }

    let json: CollaTechResponse<T>;
    try {
      json = (await res.json()) as CollaTechResponse<T>;
    } catch {
      this.error(`[CollaTech] Invalid JSON response`);
      throw new ApiError(res.status, "Invalid JSON response from agent");
    }

    this.log(`[CollaTech] Response OK:`, json);

    if (!json.ok) {
      throw new ApiError(res.status, json.error ?? "Unknown error");
    }

    return json.data as T;
  }

  private log(message?: unknown, ...optionalParams: unknown[]): void {
    if (this.debug) console.log(message, ...optionalParams);
  }

  private error(message?: unknown, ...optionalParams: unknown[]): void {
    if (this.debug) console.error(message, ...optionalParams);
  }
}

// ---------------------------------------------------------------------------
// Print Job Builder (fluent API)
// ---------------------------------------------------------------------------

type LineOptions = {
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
};

/**
 * Fluent builder for constructing print jobs.
 * Created via {@link CollaTech.createJob}.
 */
export class PrintJobBuilder {
  private readonly client: CollaTech;
  private readonly printer: string;
  private _title?: string;
  private _lines: TicketLine[] = [];
  private _qr?: string;
  private _barcode?: string;
  private _logo?: string;
  private _cut: CutMode | boolean = true;
  private _drawer = false;
  private _compact?: boolean;
  private _upsideDown?: boolean;
  private _font?: PrinterFont;
  private _feedTop?: number;
  private _feedBottom?: number;
  private _width?: PaperWidth;
  private _scale?: number;

  constructor(client: CollaTech, printer: string) {
    this.client = client;
    this.printer = printer;
  }

  /** Set the ticket title (printed large & centered). */
  title(text: string): this {
    this._title = text;
    return this;
  }

  /** Add a text line to the ticket. */
  line(text: string, options?: LineOptions): this {
    this._lines.push({ text, ...options });
    return this;
  }

  /** Add a blank line (or several). */
  blank(count = 1): this {
    this._lines.push({ text: "", gap: Math.max(0, count - 1) });
    return this;
  }

  /** Add a framed line. */
  box(text: string, options?: Omit<LineOptions, "box">): this {
    return this.line(text, { ...options, box: true });
  }

  /**
   * Add a table. Each row must have at most as many cells as columns; the
   * extra ones are ignored by the agent.
   */
  table(table: TicketTable): this {
    this._lines.push({ type: "table", table });
    return this;
  }

  /** Add a bold line. */
  boldLine(text: string, options?: Omit<LineOptions, "bold">): this {
    return this.line(text, { ...options, bold: true });
  }

  /** QR con tamano y correccion a medida. */
  qrCode(spec: QRSpec, options?: Pick<LineOptions, "align">): this {
    if (spec.data.length > MAX_QR_LEN) {
      throw new ValidationError(
        `El QR admite ${MAX_QR_LEN} caracteres como maximo, se pasaron ${spec.data.length}`,
        "qr"
      );
    }
    this._lines.push({ type: "qr", qr: spec, ...options });
    return this;
  }

  /** Codigo de barras con tipo, alto y grosor a medida. */
  barcodeCode(spec: BarcodeSpec, options?: Pick<LineOptions, "align">): this {
    if (spec.data.length > MAX_BARCODE_LEN) {
      throw new ValidationError(
        `El codigo de barras admite ${MAX_BARCODE_LEN} caracteres como maximo, se pasaron ${spec.data.length}`,
        "barcode"
      );
    }
    this._lines.push({ type: "barcode", barcode: spec, ...options });
    return this;
  }

  /** Add a QR code. Up to 2953 characters. */
  qr(data: string): this {
    if (data.length > MAX_QR_LEN) {
      throw new ValidationError(
        `El QR admite ${MAX_QR_LEN} caracteres como maximo, se pasaron ${data.length}`,
        "qr"
      );
    }
    this._qr = data;
    return this;
  }

  /** Add a barcode. Up to 253 characters (Code128). */
  barcode(data: string): this {
    if (data.length > MAX_BARCODE_LEN) {
      throw new ValidationError(
        `El codigo de barras admite ${MAX_BARCODE_LEN} caracteres como maximo, se pasaron ${data.length}`,
        "barcode"
      );
    }
    this._barcode = data;
    return this;
  }

  /** Set a base64 logo image. */
  logo(base64: string): this {
    this._logo = base64;
    return this;
  }

  /** Ancho del papel en PUNTOS: 384 (58 mm), 512 (72 mm) o 576 (80 mm). */
  width(px: PaperWidth): this {
    this._width = px;
    return this;
  }

  /** Set image scale (35–100). */
  scale(pct: number): this {
    this._scale = pct;
    return this;
  }

  /** Modo de corte: "partial", "full", "none", o true/false. */
  cut(value: CutMode | boolean): this {
    this._cut = value;
    return this;
  }

  /** Aprieta el interlineado: alrededor de un 20% menos de papel. */
  compact(value = true): this {
    this._compact = value;
    return this;
  }

  /** Imprime el ticket girado 180 grados. */
  upsideDown(value = true): this {
    this._upsideDown = value;
    return this;
  }

  /** Fuente interna: "a" normal, "b" condensada. */
  font(value: PrinterFont): this {
    this._font = value;
    return this;
  }

  /** Lineas en blanco al principio (0 por defecto) y antes del corte. */
  feed(top?: number, bottom?: number): this {
    if (top !== undefined) this._feedTop = top;
    if (bottom !== undefined) this._feedBottom = bottom;
    return this;
  }

  /** Linea separadora del ancho del papel. */
  rule(char = "-"): this {
    this._lines.push({ type: "rule", rule: char });
    return this;
  }

  /** Imagen en base64 o data URI. */
  image(data: string, options?: Pick<LineOptions, "align">): this {
    this._lines.push({ type: "image", image: data, ...options });
    return this;
  }

  /** Whether to open the cash drawer. */
  drawer(value: boolean): this {
    this._drawer = value;
    return this;
  }

  /** Send the print job to the agent. */
  async print(): Promise<PrintResult> {
    return this.client.printTicket({
      printer: this.printer,
      title: this._title,
      lines: this._lines,
      qr: this._qr,
      barcode: this._barcode,
      logo: this._logo,
      width: this._width,
      scale: this._scale,
      cut: this._cut,
      drawer: this._drawer,
      compact: this._compact,
      upside_down: this._upsideDown,
      font: this._font,
      feed_top: this._feedTop,
      feed_bottom: this._feedBottom,
    });
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function isAbortError(err: unknown): boolean {
  // En algunos runtimes de Node/edge DOMException no esta en el ambito global,
  // asi que comprobar el instanceof degradaba el timeout a un error generico.
  return typeof err === "object" && err !== null && (err as { name?: string }).name === "AbortError";
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
