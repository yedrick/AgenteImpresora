// Pruebas del SDK ya compilado (dist/), sin agente: un fetch falso hace de
// agente y de navegador. Se ejecutan con "npm test".
import { test } from "node:test";
import assert from "node:assert/strict";
import { CollaTech, ConnectionError } from "../dist/esm/index.js";

// fetchDeNavegador imita la regla del navegador: fetch tiene que llamarse
// suelto (o sobre window), nunca como metodo de otro objeto. Si no, lanza
// "Illegal invocation", que es lo que dejaba al SDK sin funcionar en
// cualquier web mientras en Node iba bien.
function fetchDeNavegador(respuesta) {
  return function (input) {
    if (this !== undefined && this !== globalThis) {
      throw new TypeError("Illegal invocation");
    }
    return Promise.resolve(
      new Response(JSON.stringify(respuesta(String(input))), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    );
  };
}

test("las peticiones funcionan con la regla de fetch del navegador", async () => {
  const sdk = new CollaTech({
    baseUrl: "http://127.0.0.1:18743",
    fetch: fetchDeNavegador(() => ({ ok: true, data: [{ name: "caja", ready: true, targets: [] }] })),
  });
  const estaciones = await sdk.stations();
  assert.equal(estaciones[0].name, "caja");
});

test("el fetch global del navegador tambien", async () => {
  const original = globalThis.fetch;
  globalThis.fetch = fetchDeNavegador(() => ({ ok: true, message: "healthy" }));
  try {
    assert.equal(await new CollaTech().isReady(), true);
  } finally {
    globalThis.fetch = original;
  }
});

test("station y job usan la ruta con el nombre escapado", async () => {
  const pedidas = [];
  const sdk = new CollaTech({
    fetch: fetchDeNavegador((url) => {
      pedidas.push(url);
      return { ok: true, data: { id: "1", status: "Completed" } };
    }),
  });
  await sdk.station("barra norte");
  await sdk.job("2026/1");
  assert.deepEqual(pedidas, [
    "http://localhost:18743/api/stations/barra%20norte",
    "http://localhost:18743/api/jobs/2026%2F1",
  ]);
});

test("sin conexion da ConnectionError", async () => {
  const sdk = new CollaTech({ fetch: () => Promise.reject(new TypeError("Failed to fetch")) });
  await assert.rejects(sdk.health(), ConnectionError);
});
