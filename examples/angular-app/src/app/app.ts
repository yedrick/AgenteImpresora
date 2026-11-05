import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { CollatechService } from './collatech.service';

interface ResultadoPrueba {
  impresora: string;
  ok: boolean;
  texto: string;
}

@Component({
  selector: 'app-root',
  imports: [FormsModule],
  templateUrl: './app.html',
  styleUrl: './app.css',
})
export class App {
  protected readonly agente = inject(CollatechService);

  // Copia editable del formulario: solo se aplica al pulsar Conectar.
  protected url = this.agente.url();
  protected key = this.agente.key();
  protected verKey = false;

  protected readonly imprimiendo = signal<string | null>(null);
  protected readonly resultado = signal<ResultadoPrueba | null>(null);
  protected readonly refrescando = signal(false);

  protected async conectar(): Promise<void> {
    this.resultado.set(null);
    await this.agente.conectar(this.url, this.key);
  }

  protected async refrescar(): Promise<void> {
    this.refrescando.set(true);
    try {
      await this.agente.refrescar();
    } finally {
      this.refrescando.set(false);
    }
  }

  protected async probar(impresora: string): Promise<void> {
    this.imprimiendo.set(impresora);
    this.resultado.set(null);
    try {
      const trabajo = await this.agente.imprimirPrueba(impresora);
      const ok = trabajo.status === 'Completed';
      this.resultado.set({
        impresora,
        ok,
        texto: ok
          ? 'Impreso. Revisa el ticket en la impresora.'
          : trabajo.error || `El trabajo quedo en estado ${trabajo.status}.`,
      });
    } catch (e) {
      this.resultado.set({ impresora, ok: false, texto: (e as Error).message });
    } finally {
      this.imprimiendo.set(null);
    }
  }

  protected copiar(texto: string): void {
    navigator.clipboard?.writeText(texto).catch(() => {});
  }
}
