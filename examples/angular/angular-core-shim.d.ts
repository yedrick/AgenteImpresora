// Lo justo de @angular/core para que el servicio de ejemplo y los bloques de
// ANGULAR.md se puedan comprobar con tsc sin arrastrar Angular entero. En tu
// proyecto esto no hace falta: lo trae @angular/core de verdad.
declare module '@angular/core' {
  export function Injectable(metadata?: unknown): ClassDecorator;
  export function Component(metadata?: unknown): ClassDecorator;
  export function NgModule(metadata?: unknown): ClassDecorator;
  export function Input(nombre?: string): PropertyDecorator;
  export function Output(nombre?: string): PropertyDecorator;
  export class EventEmitter<T> {
    emit(valor?: T): void;
    subscribe(fn: (valor: T) => void): { unsubscribe(): void };
  }
}
