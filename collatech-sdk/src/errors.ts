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

/** Thrown when the agent rejects the request (e.g. not localhost). */
export class ForbiddenError extends CollaTechError {
  constructor(message = "Request blocked by CollaTech Agent (localhost only)") {
    super(message);
    this.name = "ForbiddenError";
  }
}
