import { Injectable, computed, signal } from '@angular/core';
import {
  ApiError,
  CollaTech,
  ConnectionError,
  PrintJob,
  PrintResult,
  PrinterInfo,
  StationState,
  UnauthorizedError,
} from 'collatech-sdk';

/**
 * Como esta la conexion con el agente. Cada estado tiene su arreglo, y la
 * pantalla se lo dice al usuario en vez de un "error de conexion" generico:
 *
 * - apagado:            el agente no responde en esa direccion.
 * - web-no-autorizada:  el agente responde, pero el navegador bloquea la
 *                       respuesta porque esta web no esta en "Webs autorizadas".
 * - key-incorrecta:     el agente pide la key (token) y falta o no coincide.
 */
export type EstadoConexion =
  | 'sin-conectar'
  | 'conectando'
  | 'conectado'
  | 'apagado'
  | 'web-no-autorizada'
  | 'key-incorrecta'
  | 'error';

const CLAVE_GUARDADA = 'collatech.conexion';
const URL_POR_DEFECTO = 'http://localhost:18743';

@Injectable({ providedIn: 'root' })
export class CollatechService {
  private cliente: CollaTech | null = null;

  readonly url = signal(URL_POR_DEFECTO);
  readonly key = signal('');
  readonly estado = signal<EstadoConexion>('sin-conectar');
  readonly detalle = signal('');
  readonly impresoras = signal<PrinterInfo[]>([]);
  readonly estaciones = signal<StationState[]>([]);

  readonly conectado = computed(() => this.estado() === 'conectado');

  /**
   * El agente NO pide la key a su propia PC: solo a las peticiones que
   * llegan desde otra maquina de la red. Si la URL es localhost, la key da
   * igual, y conviene decirlo para que nadie piense que la valido.
   */
  readonly esLaMismaPC = computed(() => {
    try {
      const host = new URL(this.url()).hostname;
      return host === 'localhost' || host === '127.0.0.1' || host === '[::1]';
    } catch {
      return false;
    }
  });

  constructor() {
    try {
      const guardada = JSON.parse(localStorage.getItem(CLAVE_GUARDADA) ?? 'null');
      if (guardada?.url) this.url.set(guardada.url);
      if (guardada?.key) this.key.set(guardada.key);
    } catch {
      // Sin localStorage (modo privado, por ejemplo) se empieza de cero.
    }
  }

  /** Conecta con el agente y deja dicho por que no se pudo, si falla. */
  async conectar(url: string, key: string): Promise<void> {
    const limpia = url.trim().replace(/\/+$/, '') || URL_POR_DEFECTO;
    this.url.set(limpia);
    this.key.set(key.trim());
    this.guardar();

    this.estado.set('conectando');
    this.detalle.set('');
    this.cliente = new CollaTech({ baseUrl: limpia, token: this.key(), timeout: 8000 });

    try {
      // /health no pasa por la key; /api/status si. Por eso se prueba con
      // status: si responde, la key (cuando hace falta) es buena.
      await this.cliente.status();
      this.estado.set('conectado');
      await this.refrescar();
    } catch (e) {
      await this.explicarFallo(e);
    }
  }

  desconectar(): void {
    this.cliente = null;
    this.estado.set('sin-conectar');
    this.impresoras.set([]);
    this.estaciones.set([]);
  }

  async refrescar(): Promise<void> {
    const c = this.requerido();
    const [impresoras, estaciones] = await Promise.all([
      c.printers(),
      c.stations().catch(() => [] as StationState[]),
    ]);
    this.impresoras.set(impresoras ?? []);
    this.estaciones.set(estaciones ?? []);
  }

  /** Imprime un ticket de prueba y espera a saber como salio. */
  async imprimirPrueba(impresora: string): Promise<PrintJob> {
    const c = this.requerido();
    const resultado: PrintResult = await c.printTicket({
      printer: impresora,
      title: 'PRUEBA DE IMPRESION',
      lines: [
        { text: 'Conexion desde Angular', align: 'center', bold: true },
        { type: 'rule', rule: '-' },
        { text: `Impresora: ${impresora}` },
        { text: `Fecha: ${new Date().toLocaleString()}` },
        { text: `Agente: ${this.url()}` },
        { type: 'rule', rule: '-' },
        { text: 'Si lees esto, todo funciona.', align: 'center' },
      ],
      cut: true,
    });
    const trabajo = Array.isArray(resultado) ? resultado[0] : resultado;
    return this.esperarTrabajo(trabajo);
  }

  /** Sigue un trabajo hasta que termina (o hasta 15 s). */
  private async esperarTrabajo(trabajo: PrintJob): Promise<PrintJob> {
    const c = this.requerido();
    let actual = trabajo;
    for (let i = 0; i < 30; i++) {
      if (actual.status === 'Completed' || actual.status === 'Failed') return actual;
      await new Promise((r) => setTimeout(r, 500));
      actual = await c.job(trabajo.id);
    }
    return actual;
  }

  private async explicarFallo(e: unknown): Promise<void> {
    if (e instanceof UnauthorizedError) {
      this.estado.set('key-incorrecta');
      this.detalle.set(
        this.key()
          ? 'La key no coincide con la del agente.'
          : 'Este agente pide una key porque te conectas desde otra PC.',
      );
      return;
    }
    if (e instanceof ConnectionError) {
      // El navegador da el mismo error para "agente apagado" y "web no
      // autorizada" (CORS). Una peticion no-cors a /health lo distingue:
      // si el agente esta encendido, esa peticion SI llega y resuelve.
      const responde = await fetch(`${this.url()}/health`, { mode: 'no-cors' })
        .then(() => true)
        .catch(() => false);
      if (responde) {
        this.estado.set('web-no-autorizada');
        this.detalle.set(window.location.origin);
      } else {
        this.estado.set('apagado');
        this.detalle.set(this.url());
      }
      return;
    }
    this.estado.set('error');
    this.detalle.set(e instanceof ApiError ? e.apiMessage : String((e as Error)?.message ?? e));
  }

  private requerido(): CollaTech {
    if (!this.cliente) throw new Error('Primero conecta con el agente.');
    return this.cliente;
  }

  private guardar(): void {
    try {
      localStorage.setItem(CLAVE_GUARDADA, JSON.stringify({ url: this.url(), key: this.key() }));
    } catch {
      // Sin localStorage no se recuerda, pero la conexion funciona igual.
    }
  }
}
