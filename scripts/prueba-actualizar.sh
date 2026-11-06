#!/usr/bin/env bash
# Prueba de punta a punta de "actualizar desde el panel" en Linux, con
# systemd y root de verdad. La usa CI (job actualizar-e2e); en local hace
# falta sudo y no tener el agente instalado.
#
#   1. instala el .deb 1.0.0 (compilado con la etiqueta actualizarprueba);
#   2. publica en local una 1.0.1 firmada con una clave de prueba;
#   3. pide la actualizacion por la API, como el boton del panel;
#   4. comprueba que responde la 1.0.1 y que la configuracion no cambio;
#   5. publica una 1.0.2 que no arranca y comprueba que vuelve sola a la 1.0.1.
set -euo pipefail

RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"                       # fuera del repositorio: aqui va una clave
PUBLICACION="$T/publicacion"
PUERTO_PUB=18790
AGENTE=http://127.0.0.1:18743
mkdir -p "$PUBLICACION"
cd "$RAIZ"

paso() { echo; echo "== $*"; }
falla() {
  trap - ERR
  local diag
  diag="$(
    echo "FALLO: $*"
    echo "--- journalctl"
    sudo journalctl -u collatech-agent -u collatech-agent-actualizar -u collatech-agent-actualizar.path --no-pager 2>&1 | tail -40
    echo "--- aplicar.log"
    sudo cat /var/lib/collatech/actualizacion/aplicar.log 2>&1 | tail -20
    echo "--- systemctl"
    systemctl status collatech-agent-actualizar.path --no-pager 2>&1 | tail -8
  )"
  echo "$diag" >&2
  # Los registros de Actions piden iniciar sesion para leerlos; una anotacion
  # se ve en la pagina de la ejecucion sin mas.
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    local enc="${diag//'%'/'%25'}"; enc="${enc//$'\r'/}"; enc="${enc//$'\n'/'%0A'}"
    echo "::error title=prueba-actualizar::$enc"
  fi
  exit 1
}
trap 'falla "linea $LINENO: $BASH_COMMAND"' ERR
version() { curl -sf "$AGENTE/health" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["version"])' 2>/dev/null || true; }
esperar_version() {
  local quiero="$1" limite="$2"
  for _ in $(seq "$limite"); do
    [ "$(version)" = "$quiero" ] && return 0
    sleep 1
  done
  return 1
}
publicar() {   # publicar <version>: el manifiesto y latest.json para lo que haya en $PUBLICACION
  local v="$1"
  go run ./cmd/firmar manifiesto -dir "$PUBLICACION" -version "$v" >/dev/null
  FIRMA_ACTUALIZACIONES="$(cat "$T/clave/privada.txt")" go run ./cmd/firmar firmar "$PUBLICACION/manifiesto.json" >/dev/null
  cat > "$PUBLICACION/latest.json" <<EOF
{"tag_name":"v$v","html_url":"http://127.0.0.1:$PUERTO_PUB/notas","assets":[
 {"name":"manifiesto.json","browser_download_url":"http://127.0.0.1:$PUERTO_PUB/manifiesto.json"},
 {"name":"manifiesto.json.sig","browser_download_url":"http://127.0.0.1:$PUERTO_PUB/manifiesto.json.sig"},
 {"name":"actualizacion-linux-amd64","browser_download_url":"http://127.0.0.1:$PUERTO_PUB/actualizacion-linux-amd64"}]}
EOF
}
pedir() {      # pedir <version>: buscar e instalar, como el panel
  curl -sf -X POST -H 'Content-Type: application/json' -d '{}' "$AGENTE/api/actualizacion/buscar" | grep -q "\"version\":\"$1\"" \
    || falla "buscar no ofrece la $1: $(curl -s -X POST -H 'Content-Type: application/json' -d '{}' "$AGENTE/api/actualizacion/buscar")"
  curl -sf -X POST -H 'Content-Type: application/json' -d "{\"version\":\"$1\"}" "$AGENTE/api/actualizacion/instalar" >/dev/null \
    || falla "instalar la $1 fue rechazado"
}

paso "Clave de prueba"
go run ./cmd/firmar generar "$T/clave" >/dev/null
PUB="$(cat "$T/clave/publica.txt")"

paso "Publicacion 1.0.1"
ETIQUETAS_GO=actualizarprueba ./scripts/empaquetar.sh 1.0.1 >/dev/null
cp dist/actualizacion-linux-amd64 "$PUBLICACION/"
publicar 1.0.1
(cd "$PUBLICACION" && python3 -m http.server "$PUERTO_PUB" --bind 127.0.0.1 >/dev/null 2>&1 &)

paso "Instalar el .deb 1.0.0"
ETIQUETAS_GO=actualizarprueba ./scripts/empaquetar.sh 1.0.0 >/dev/null
sudo dpkg --install dist/collatech-agent_1.0.0_amd64.deb
for u in collatech-agent collatech-agent-actualizar; do
  sudo mkdir -p "/etc/systemd/system/$u.service.d"
  printf '[Service]\nEnvironment=COLLATECH_URL_ACTUALIZACIONES=http://127.0.0.1:%s/latest.json\nEnvironment=COLLATECH_CLAVE_PRUEBA=%s\n' \
    "$PUERTO_PUB" "$PUB" | sudo tee "/etc/systemd/system/$u.service.d/prueba.conf" >/dev/null
done
sudo systemctl daemon-reload
sudo systemctl restart collatech-agent
systemctl is-active --quiet collatech-agent-actualizar.path || falla "la unidad .path no quedo activa tras instalar el .deb"
esperar_version 1.0.0 30 || falla "el agente 1.0.0 no responde"
CONFIG_ANTES="$(sudo sha256sum /etc/collatech/config.json)"

paso "Actualizar a la 1.0.1 desde la API del panel"
pedir 1.0.1
esperar_version 1.0.1 90 || falla "no se actualizo a la 1.0.1 (responde: $(version))"
[ "$(sudo sha256sum /etc/collatech/config.json)" = "$CONFIG_ANTES" ] || falla "la configuracion cambio al actualizar"
sudo grep -q '"ok": true' /var/lib/collatech/actualizacion/resultado.json || falla "resultado.json no dice ok"
echo "OK: 1.0.0 -> 1.0.1"

paso "Publicacion 1.0.2 que no arranca: tiene que volver sola a la 1.0.1"
printf '#!/bin/sh\necho "version rota" >&2\nexit 1\n' > "$PUBLICACION/actualizacion-linux-amd64"
publicar 1.0.2
pedir 1.0.2
for _ in $(seq 150); do
  sudo grep -q '"hasta": "1.0.2"' /var/lib/collatech/actualizacion/resultado.json 2>/dev/null && break
  sleep 1
done
sudo grep -q '"ok": false' /var/lib/collatech/actualizacion/resultado.json || falla "resultado.json no dice que fallo la 1.0.2"
esperar_version 1.0.1 30 || falla "no volvio a la 1.0.1 (responde: $(version))"
[ "$(sudo sha256sum /etc/collatech/config.json)" = "$CONFIG_ANTES" ] || falla "la configuracion cambio al volver atras"
systemctl is-active --quiet collatech-agent || falla "el agente quedo parado"
echo "OK: 1.0.2 rota -> vuelve a la 1.0.1"

paso "Limpieza"
sudo dpkg --purge collatech-agent >/dev/null
rm -rf "$T"
echo
echo "Prueba de actualizar desde el panel: todo bien."
