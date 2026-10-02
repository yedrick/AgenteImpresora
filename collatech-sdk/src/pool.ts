// ---------------------------------------------------------------------------
// CollaTech Agent SDK – Multi-agent pool
// ---------------------------------------------------------------------------

import { CollaTech } from "./client.js";
import { CollaTechError } from "./errors.js";
import type {
  CollaTechConfig,
  PrintResult,
  PrintTextOptions,
  PrintTicketOptions,
} from "./types.js";

/** Config for one agent inside a pool. Same shape as {@link CollaTechConfig}. */
export interface PoolAgentConfig extends CollaTechConfig {
  /** Optional human label, e.g. "Caja 1" or "PC Cocina". Purely descriptive. */
  label?: string;
}

export interface PoolHealthResult {
  name: string;
  ok: boolean;
  error?: string;
}

export interface PoolPrintResult {
  name: string;
  ok: boolean;
  result?: PrintResult;
  error?: string;
}

/**
 * Registry of several CollaTech Agent instances (one per PC/IP on the
 * network), so an app can print to multiple stations without wiring up a
 * `CollaTech` client by hand for every host.
 *
 * @example
 * ```ts
 * import { CollaTechPool } from "collatech-sdk";
 *
 * const pool = new CollaTechPool({
 *   caja: "http://192.168.1.10:18743",
 *   cocina: "http://192.168.1.11:18743",
 * });
 *
 * await pool.agent("caja").printText({ printer: "EPSON Caja", text: "Hola" });
 * await pool.printTextTo("cocina", { printer: "EPSON Cocina", text: "Comanda" });
 *
 * // Send the same ticket to every registered PC at once.
 * await pool.broadcastTicket({ printer: "default", title: "Aviso", lines: [] });
 *
 * // Check which agents are reachable right now.
 * console.log(await pool.healthAll());
 * ```
 */
export class CollaTechPool {
  private readonly agents = new Map<string, CollaTech>();

  constructor(initial?: Record<string, PoolAgentConfig | string>) {
    if (!initial) return;
    for (const [name, config] of Object.entries(initial)) {
      this.add(name, config);
    }
  }

  /** Register (or replace) an agent under `name`. Accepts a base URL string or a full config. */
  add(name: string, config: PoolAgentConfig | string): CollaTech {
    const resolved = typeof config === "string" ? { baseUrl: config } : config;
    const client = new CollaTech(resolved);
    this.agents.set(name, client);
    return client;
  }

  /** Remove a previously registered agent. Returns false if it didn't exist. */
  remove(name: string): boolean {
    return this.agents.delete(name);
  }

  /** Get the `CollaTech` client registered under `name`, for direct use of any SDK method. */
  agent(name: string): CollaTech {
    const client = this.agents.get(name);
    if (!client) {
      throw new CollaTechError(
        `No agent registered under name "${name}". Call pool.add("${name}", { baseUrl }) first.`
      );
    }
    return client;
  }

  /** Names of every registered agent. */
  list(): string[] {
    return Array.from(this.agents.keys());
  }

  /** Check health of every registered agent in parallel. */
  async healthAll(): Promise<PoolHealthResult[]> {
    return Promise.all(
      this.list().map(async (name) => {
        try {
          await this.agent(name).health();
          return { name, ok: true };
        } catch (err) {
          return { name, ok: false, error: errorMessage(err) };
        }
      })
    );
  }

  /** Print text on a single named agent. */
  printTextTo(name: string, options: PrintTextOptions): Promise<PrintResult> {
    return this.agent(name).printText(options);
  }

  /** Print a ticket on a single named agent. */
  printTicketTo(name: string, options: PrintTicketOptions): Promise<PrintResult> {
    return this.agent(name).printTicket(options);
  }

  /** Send the same ticket to every registered agent at once (e.g. cocina + barra + caja). */
  async broadcastTicket(options: PrintTicketOptions): Promise<PoolPrintResult[]> {
    return Promise.all(
      this.list().map(async (name) => {
        try {
          const result = await this.agent(name).printTicket(options);
          return { name, ok: true, result };
        } catch (err) {
          return { name, ok: false, error: errorMessage(err) };
        }
      })
    );
  }

  /** Send the same plain text to every registered agent at once. */
  async broadcastText(options: PrintTextOptions): Promise<PoolPrintResult[]> {
    return Promise.all(
      this.list().map(async (name) => {
        try {
          const result = await this.agent(name).printText(options);
          return { name, ok: true, result };
        } catch (err) {
          return { name, ok: false, error: errorMessage(err) };
        }
      })
    );
  }
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
