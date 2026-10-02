// ---------------------------------------------------------------------------
// CollaTech Agent SDK – Error Classes
// ---------------------------------------------------------------------------

/** Base error for all CollaTech SDK errors. */
export class CollaTechError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CollaTechError";
  }
}

/** Thrown when the agent is unreachable or times out. */
export class ConnectionError extends CollaTechError {
  constructor(message = "Cannot connect to CollaTech Agent") {
    super(message);
    this.name = "ConnectionError";
  }
}

/** Thrown when the API returns a non-2xx status. */
export class ApiError extends CollaTechError {
  public readonly status: number;
  public readonly apiMessage: string;

  constructor(status: number, message: string) {
    super(`CollaTech API error ${status}: ${message}`);
    this.name = "ApiError";
    this.status = status;
    this.apiMessage = message;
  }
}

/** Thrown when the request payload is invalid. */
export class ValidationError extends CollaTechError {
  public readonly field?: string;

  constructor(message: string, field?: string) {
    super(message);
    this.name = "ValidationError";
    this.field = field;
  }
}

/** Se lanza cuando el endpoint solo se atiende desde la propia PC del agente. */
export class ForbiddenError extends CollaTechError {
  constructor(
    message = "Este endpoint solo responde desde la PC donde corre el agente (diagnostico, logs y cambios de configuracion)."
  ) {
    super(message);
    this.name = "ForbiddenError";
  }
}

/** Se lanza cuando el agente exige un token y falta o no coincide. */
export class UnauthorizedError extends CollaTechError {
  constructor(
    message = "El agente pide un token de acceso. Pasalo en new CollaTech({ token }). Lo muestra el panel local del agente."
  ) {
    super(message);
    this.name = "UnauthorizedError";
  }
}

/** Se lanza cuando la cola del agente esta llena (impresora caida o saturada). */
export class QueueFullError extends CollaTechError {
  constructor(
    message = "La cola de impresion del agente esta llena. Revisa si la impresora responde."
  ) {
    super(message);
    this.name = "QueueFullError";
  }
}
