// ---------------------------------------------------------------------------
// CollaTech Agent SDK – Public API
// ---------------------------------------------------------------------------
//
//  Install:
//    npm install collatech-sdk
//
//  Quick start:
//    import { CollaTech } from "collatech-sdk";
//    const printer = new CollaTech();
//    await printer.printText({ printer: "Impresora1", text: "Hola!" });
//
// ---------------------------------------------------------------------------

export { CollaTech, PrintJobBuilder, MAX_QR_LEN, MAX_BARCODE_LEN } from "./client.js";
export { CollaTechPool } from "./pool.js";
export {
  CollaTechError,
  ConnectionError,
  ApiError,
  ValidationError,
  ForbiddenError,
  UnauthorizedError,
  QueueFullError,
} from "./errors.js";
export { fileToBase64, toBase64, fromBase64, stripDataUri } from "./utils.js";

// Re-export all types
export type {
  CollaTechConfig,
  CollaTechResponse,
  HealthData,
  StatusData,
  PrintJob,
  PrintResult,
  WaitUntilReadyOptions,
  NetworkInfo,
  PrinterInfo,
  PrinterAlias,
  Settings,
  TemplateName,
  TemplateData,
  TemplateItem,
  PrintInvoiceOptions,
  PrintTextOptions,
  PrintTicketOptions,
  TicketLine,
  TextAlign,
  PrintTemplateOptions,
  PrintHTMLOptions,
  PrintImageOptions,
  PrintLogoOptions,
  PrintRawOptions,
  LogEntry,
} from "./types.js";
export type { PoolAgentConfig, PoolHealthResult, PoolPrintResult } from "./pool.js";
