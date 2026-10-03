// Comprueba que el conversor de Markdown de la pagina /docs produce el HTML
// esperado a partir de docs/UBUNTU.md.
//
// El conversor vive dentro de internal/api/web/docs.html, no en un archivo
// propio, asi que aqui se extrae de la pagina tal cual: la prueba corre
// sobre el codigo de verdad y no sobre una copia que se desincroniza.
//
//   node examples/comprobar-guia.js
"use strict";

const fs = require("fs");
const path = require("path");

const raiz = path.join(__dirname, "..");
const pagina = fs.readFileSync(path.join(raiz, "internal/api/web/docs.html"), "utf8");

const ini = pagina.indexOf("(function () {");
const fin = pagina.indexOf('  fetch("/docs/ubuntu.md")');
if (ini < 0 || fin < 0) {
  console.error("No se encontro el conversor dentro de docs.html: cambio la pagina.");
  process.exit(1);
}
const aHTML = eval(pagina.slice(ini, fin) + "  return aHTML;\n})()");

const md = fs.readFileSync(path.join(raiz, "docs/UBUNTU.md"), "utf8");
const html = aHTML(md);

const comprobar = [
  ["titulo h1",         /<h1>Conectar una impresora termica en Ubuntu<\/h1>/],
  ["secciones h2",      /<h2>Camino corto/],
  ["bloque de codigo",  /<pre><code>lsusb \| grep -i epson<\/code><\/pre>/],
  ["tabla con th",      /<table><tr><th>Descarga de Epson<\/th>/],
  ["celda de tabla",    /<td>Solo Windows<\/td>/],
  ["lista ordenada",    /<ol><li>Papel, tapa cerrada/],
  ["negrita",           /<strong>no hace falta ninguna<\/strong>/],
  ["codigo en linea",   /<code>04b8:0202<\/code>/],
  ["enlace",            /<a href="http:\/\/localhost:18743\/impresoras">/],
];

let mal = 0;
for (const [que, re] of comprobar) {
  const ok = re.test(html);
  if (!ok) mal++;
  console.log(`  ${ok ? "ok " : "MAL"}  ${que}`);
}

// Que no sobreviva Markdown sin convertir. Se quitan antes los bloques y los
// trozos de codigo: ahi un '#' es un comentario de bash y los '***' del
// mensaje del instalador de Epson son texto literal; los dos deben quedarse.
const sinCodigo = html
  .replace(/<pre>[\s\S]*?<\/pre>/g, "")
  .replace(/<code>[\s\S]*?<\/code>/g, "");
const restos = sinCodigo.match(/^#{1,4}\s|\*\*|^\|/m);
if (restos) mal++;
console.log(`  ${restos ? "MAL" : "ok "}  sin markdown sin convertir${restos ? ": " + restos[0] : ""}`);

console.log(`\n  ${html.length} bytes de HTML, ${mal} comprobaciones fallidas`);
process.exit(mal ? 1 : 0);
