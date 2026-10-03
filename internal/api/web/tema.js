/* Menu y selector de tema, compartidos por las tres paginas.
 *
 * El tema se aplica a <html> con data-theme. El valor se lee y se escribe en
 * localStorage, que es por navegador: no se manda al agente ni se comparte
 * entre PCs, y si el navegador lo bloquea se usa "claro" y no pasa nada. */
(function () {
  "use strict";

  var CLAVE = "collatech-tema";
  var VALIDOS = ["claro", "oscuro", "auto"];

  function leer() {
    try {
      var v = localStorage.getItem(CLAVE);
      return VALIDOS.indexOf(v) >= 0 ? v : "claro";
    } catch (e) {
      // Ventana privada o almacenamiento bloqueado: no es un error.
      return "claro";
    }
  }

  function aplicar(tema) {
    document.documentElement.setAttribute("data-theme", tema);
    try { localStorage.setItem(CLAVE, tema); } catch (e) { /* da igual */ }
    document.querySelectorAll(".ct-tema button").forEach(function (b) {
      b.setAttribute("aria-pressed", String(b.dataset.tema === tema));
    });
  }

  // Se aplica ya, antes de pintar, para que no se vea el cambio de color.
  aplicar(leer());

  var PAGINAS = [
    { href: "/panel", texto: "Panel" },
    { href: "/impresoras", texto: "Impresoras" },
    { href: "/designer", texto: "Disenador" },
    { href: "/sdk", texto: "SDK" },
    { href: "/diagnostico", texto: "Diagnostico" },
  ];

  function montar() {
    var hueco = document.getElementById("ct-menu");
    if (!hueco) return;

    var aqui = location.pathname === "/" ? "/panel" : location.pathname;
    var html = '<a class="ct-marca" href="/panel">' +
      '<span class="ct-punto" id="ct-salud" title="Estado del agente"></span>' +
      'CollaTech Agent</a>';

    PAGINAS.forEach(function (p) {
      html += '<a class="ct-ir" href="' + p.href + '"' +
        (p.href === aqui ? ' aria-current="page"' : "") + ">" + p.texto + "</a>";
    });

    html += '<span class="ct-hueco"></span>' +
      '<div class="ct-tema" role="group" aria-label="Tema de color">' +
      '<button type="button" data-tema="claro"  title="Tema claro">Claro</button>' +
      '<button type="button" data-tema="oscuro" title="Tema oscuro">Oscuro</button>' +
      '<button type="button" data-tema="auto"   title="Seguir al sistema">Auto</button>' +
      "</div>";

    hueco.className = "ct-menu";
    hueco.innerHTML = html;

    hueco.querySelectorAll(".ct-tema button").forEach(function (b) {
      b.addEventListener("click", function () { aplicar(b.dataset.tema); });
    });
    aplicar(leer());
    latido();
  }

  // El punto de la marca dice si el agente responde, en todas las paginas.
  function latido() {
    var punto = document.getElementById("ct-salud");
    if (!punto) return;
    var ver = function () {
      fetch("/health", { cache: "no-store" })
        .then(function (r) { return r.ok ? "ok" : "mal"; })
        .catch(function () { return "mal"; })
        .then(function (e) {
          punto.dataset.estado = e;
          punto.title = e === "ok" ? "El agente responde" : "El agente no responde";
        });
    };
    ver();
    setInterval(ver, 15000);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", montar);
  } else {
    montar();
  }
})();
