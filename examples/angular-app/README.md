# Angular conectado a CollaTech Agent

Aplicación Angular de prueba: pones la dirección del agente y la key, y te
dice si conecta, lista las impresoras e imprime un ticket de prueba. Si no
conecta, te dice **por qué** y qué hacer.

## Probarla ya

```bash
cd examples/angular-app
npm install
npx ng serve --port 4300
```

Abre http://localhost:4300 y pulsa **Conectar**.

La primera vez saldrá *"El agente responde, pero esta web no está
autorizada"*. Es normal: en la PC del agente abre
http://localhost:18743/panel → **Webs autorizadas** → añade
`http://localhost:4300` → guarda, y vuelve a pulsar **Conectar**.

## Las 3 cosas que necesita una web para hablar con el agente

| | Qué es | Dónde se saca |
|---|---|---|
| **Dirección** | `http://localhost:18743` en la misma PC; `http://IP-DEL-AGENTE:18743` desde otra | Panel del agente |
| **Web autorizada** | El origen exacto de tu web (`http://localhost:4200`, `https://mi-pos.com`…) | Panel → *Webs autorizadas* |
| **Key** | El token de acceso. **Solo se pide desde otra PC**; en la misma PC no hace falta | Lo muestra el instalador al terminar y el panel → *Estado* |

## Qué significa cada estado

| La pantalla dice | Qué pasa | Arreglo |
|---|---|---|
| Conectado | Todo bien | — |
| El agente no responde | Apagado, IP equivocada o firewall | Arranca el agente; abre el puerto 18743 |
| Esta web no está autorizada | El agente responde pero el navegador bloquea la respuesta (CORS) | Añade el origen que muestra la pantalla en *Webs autorizadas* |
| Key incorrecta o vacía | Te conectas desde otra PC sin la key correcta | Cópiala del panel → *Estado* |

## Llevarlo a tu proyecto Angular

1. Instala el SDK:

   ```bash
   npm install https://github.com/yedrick/AgenteImpresora/releases/download/v1.6.2/collatech-sdk-1.6.2.tgz
   ```

2. Copia `src/app/collatech.service.ts` a tu proyecto. Es todo lo que hace
   falta para conectar, saber por qué falla, listar impresoras e imprimir.

3. Úsalo en cualquier componente:

   ```typescript
   import { Component, inject } from '@angular/core';
   import { CollatechService } from './collatech.service';

   @Component({
     selector: 'app-caja',
     template: `
       @if (agente.conectado()) {
         <button (click)="imprimir()">Imprimir</button>
       } @else {
         <p>Impresora no disponible: {{ agente.estado() }}</p>
       }
     `,
   })
   export class CajaComponent {
     protected readonly agente = inject(CollatechService);

     async ngOnInit() {
       // En la misma PC la key va vacía. Desde otra PC, pon la del panel.
       await this.agente.conectar('http://localhost:18743', '');
     }

     async imprimir() {
       const trabajo = await this.agente.imprimirPrueba('caja');
       console.log(trabajo.status); // "Completed" o "Failed"
     }
   }
   ```

La key se guarda en el `localStorage` del navegador para no tener que
escribirla cada vez. No la pongas en el código fuente si la web es pública.
