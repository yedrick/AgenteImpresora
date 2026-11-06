#!/usr/bin/env bash
# Construye los binarios de todas las plataformas y los empaqueta para
# publicar. Lo usan igual el Makefile y GitHub Actions, de forma que lo que
# se publica se puede reproducir en local con un solo comando.
#
#   scripts/empaquetar.sh 1.5.0
set -euo pipefail

VERSION="${1:-dev}"
VERSION="${VERSION#v}"            # admite tanto 1.5.0 como v1.5.0

# Debian exige que la version empiece por un digito, asi que una version de
# prueba como "ci-075e965" o "dev" tumbaba dpkg-deb. Para el .deb se le pone
# delante un 0.0.0+; los demas paquetes conservan el nombre legible.
case "$VERSION" in
  [0-9]*) VERSION_DEB="$VERSION" ;;
  *)      VERSION_DEB="0.0.0+${VERSION}" ;;
esac
RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
SALIDA="$RAIZ/dist"
LDFLAGS="-s -w -X main.version=$VERSION"
# Solo para la prueba de punta a punta de CI (scripts/prueba-actualizar.sh):
# ETIQUETAS_GO=actualizarprueba compila con los ganchos de prueba. Lo que se
# publica nunca lo lleva.
ETIQUETAS="${ETIQUETAS_GO:-}"

cd "$RAIZ"
rm -rf "$SALIDA"
mkdir -p "$SALIDA"

# --- Binarios ---------------------------------------------------------------
compilar() {
  local os="$1" arch="$2" nombre="$3" paquete="${4:-./cmd/server}"
  echo "  $nombre"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -tags "$ETIQUETAS" -ldflags "$LDFLAGS" -o "$SALIDA/$nombre" "$paquete"
}

echo "Compilando $VERSION:"
compilar linux   amd64 collatech-agent-linux-amd64
compilar linux   arm64 collatech-agent-linux-arm64
compilar darwin  amd64 collatech-agent-macos-intel
compilar darwin  arm64 collatech-agent-macos-apple
compilar windows amd64 CollaTechAgent-windows-amd64.exe
compilar windows 386   CollaTechAgent-windows-386.exe

# El instalador de Windows lleva el agente dentro. Hay que compilar antes
# CollaTechAgent.bin y pasar la etiqueta "embedagent", igual que hace
# BUILD_INSTALLER.bat. Sin eso el instalador compila igual, pero al
# ejecutarlo dice "no hay CollaTechAgent.exe disponible": asi salio la 1.6.1.
echo "  INSTALADOR-windows-amd64.exe (con el agente dentro)"
EMBEBIDO="$RAIZ/cmd/installer/CollaTechAgent.bin"
trap 'rm -f "$EMBEBIDO"' EXIT
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -tags "$ETIQUETAS" -ldflags "$LDFLAGS -H windowsgui" -o "$EMBEBIDO" ./cmd/server
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -trimpath -tags embedagent -ldflags "$LDFLAGS -H windowsgui" \
  -o "$SALIDA/INSTALADOR-windows-amd64.exe" ./cmd/installer
rm -f "$EMBEBIDO"

# Que no vuelva a salir un instalador vacio: tiene que pesar mas que el
# agente que lleva dentro.
tam_agente=$(stat -c %s "$SALIDA/CollaTechAgent-windows-amd64.exe")
tam_instalador=$(stat -c %s "$SALIDA/INSTALADOR-windows-amd64.exe")
if [ "$tam_instalador" -le "$tam_agente" ]; then
  echo "ERROR: el instalador ($tam_instalador bytes) no lleva el agente dentro ($tam_agente bytes)." >&2
  exit 1
fi

# --- Ejecutables para actualizar desde el panel ------------------------------
# El boton del panel descarga solo el ejecutable de su sistema, comprobado
# contra manifiesto.json, que GitHub Actions firma al publicar (ver
# docs/ACTUALIZAR.md). Los de Windows llevan -H windowsgui, como el que mete
# el instalador, para que una actualizacion no cambie como se comporta.
echo "  actualizacion-* y manifiesto.json"
for par in "linux:amd64:" "linux:arm64:" "windows:amd64:.exe" "windows:386:.exe"; do
  os="${par%%:*}"; resto="${par#*:}"; arch="${resto%%:*}"; ext="${resto#*:}"
  extra=""; [ "$os" = windows ] && extra=" -H windowsgui"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -tags "$ETIQUETAS" -ldflags "$LDFLAGS$extra" -o "$SALIDA/actualizacion-$os-$arch$ext" ./cmd/server
done
go run ./cmd/firmar manifiesto -dir "$SALIDA" -version "$VERSION" >/dev/null

# --- Lo que acompana a cada paquete -----------------------------------------
comunes() {
  local destino="$1"
  mkdir -p "$destino/configs" "$destino/docs"
  cp README.md JSON-REFERENCIA.md ANGULAR.md "$destino/docs/"
  cp docs/API.md "$destino/docs/" 2>/dev/null || true
  # Config de partida abierta a la red (como el instalador de Windows) y sin
  # token: el agente genera uno al arrancar, asi que desde la red nunca se
  # imprime sin el. Se cierra desde el panel: Estado -> Acceso en red.
  cat > "$destino/configs/config.json" <<'EOF'
{
  "host": "0.0.0.0",
  "port": 18743,
  "allowed_cors": ["http://localhost:18743"],
  "allow_remote": true,
  "auth_token": "",
  "max_print_size": 2097152,
  "queue": { "workers": 4, "max_retries": 2 }
}
EOF
}

echo "Empaquetando:"

# --- Windows: zip con el agente, el instalador y los scripts ----------------
tmp="$SALIDA/.tmp-win"; rm -rf "$tmp"; mkdir -p "$tmp"
cp "$SALIDA/CollaTechAgent-windows-amd64.exe" "$tmp/CollaTechAgent.exe"
cp "$SALIDA/INSTALADOR-windows-amd64.exe" "$tmp/INSTALADOR.exe"
cp "$SALIDA/CollaTechAgent-windows-386.exe" "$tmp/CollaTechAgent-32bits.exe"
cp ./*.bat "$tmp/" 2>/dev/null || true
comunes "$tmp"
(cd "$tmp" && zip -qr "$SALIDA/CollaTechAgent-$VERSION-windows.zip" .)
rm -rf "$tmp"; echo "  CollaTechAgent-$VERSION-windows.zip"

# --- Linux y macOS: tar.gz --------------------------------------------------
for par in "linux-amd64:collatech-agent-linux-amd64" \
           "linux-arm64:collatech-agent-linux-arm64" \
           "macos-intel:collatech-agent-macos-intel" \
           "macos-apple:collatech-agent-macos-apple"; do
  etiqueta="${par%%:*}"; binario="${par##*:}"
  tmp="$SALIDA/.tmp-$etiqueta"; rm -rf "$tmp"; mkdir -p "$tmp"
  cp "$SALIDA/$binario" "$tmp/collatech-agent"
  chmod +x "$tmp/collatech-agent"
  comunes "$tmp"
  cat > "$tmp/INSTALAR.sh" <<'EOF'
#!/usr/bin/env bash
# Instala el agente como servicio del sistema y lo arranca.
set -e
cd "$(dirname "$0")"
if [ "$(id -u)" -ne 0 ]; then
  echo "Hace falta root. Prueba:  sudo ./INSTALAR.sh"
  exit 1
fi
./collatech-agent --install
echo
echo "Listo. El panel esta en http://localhost:18743/panel"
EOF
  chmod +x "$tmp/INSTALAR.sh"
  (cd "$tmp" && tar czf "$SALIDA/CollaTechAgent-$VERSION-$etiqueta.tar.gz" .)
  rm -rf "$tmp"; echo "  CollaTechAgent-$VERSION-$etiqueta.tar.gz"
done

# --- Debian y Ubuntu: .deb --------------------------------------------------
# Se arma a mano en vez de con fpm para no depender de Ruby en el runner.
for par in "amd64:collatech-agent-linux-amd64" "arm64:collatech-agent-linux-arm64"; do
  arch="${par%%:*}"; binario="${par##*:}"
  pkg="$SALIDA/.deb-$arch"; rm -rf "$pkg"
  mkdir -p "$pkg/DEBIAN" "$pkg/usr/local/bin" \
           "$pkg/var/lib/collatech" "$pkg/lib/systemd/system" \
           "$pkg/usr/share/doc/collatech-agent"
  cp "$SALIDA/$binario" "$pkg/usr/local/bin/collatech-agent"
  chmod 755 "$pkg/usr/local/bin/collatech-agent"
  cp README.md JSON-REFERENCIA.md "$pkg/usr/share/doc/collatech-agent/"

  cat > "$pkg/lib/systemd/system/collatech-agent.service" <<'EOF'
[Unit]
Description=CollaTech Agent - impresion ESC/POS
After=network-online.target cups.service
Wants=network-online.target
# Sin limite de arranques: por defecto, 5 caidas en 10 segundos (un puerto
# ocupado al arrancar, por ejemplo) hacen que systemd se rinda y deje el
# agente apagado hasta que alguien lo note.
StartLimitIntervalSec=0

[Service]
Type=simple
ExecStart=/usr/local/bin/collatech-agent --config /etc/collatech/config.json --data-dir /var/lib/collatech
# always y no on-failure: tambien vuelve si el proceso sale "limpio".
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/collatech /etc/collatech
# El acceso a /dev/usb/lp* y /dev/ttyUSB* lo dan los grupos. No se usa
# DeviceAllow porque, al declararlo, systemd deniega todo lo demas.
SupplementaryGroups=lp dialout

[Install]
WantedBy=multi-user.target
EOF

  # Las dos piezas que aplican una actualizacion pedida desde el panel. El
  # servicio de arriba sigue encerrado: solo deja la orden en su carpeta de
  # datos; estas, fuera del encierro, hacen el cambio. Mismo texto que
  # unidadesActualizarText en cmd/server/service_unix.go.
  cat > "$pkg/lib/systemd/system/collatech-agent-actualizar.path" <<'EOF'
[Unit]
Description=CollaTech Agent - vigila ordenes de actualizacion del panel

[Path]
PathExists=/var/lib/collatech/actualizacion/orden.json

[Install]
WantedBy=multi-user.target
EOF

  cat > "$pkg/lib/systemd/system/collatech-agent-actualizar.service" <<'EOF'
[Unit]
Description=CollaTech Agent - aplica una actualizacion pedida desde el panel

[Service]
Type=oneshot
ExecStart=/usr/local/bin/collatech-agent --aplicar-actualizacion /var/lib/collatech/actualizacion/orden.json --config /etc/collatech/config.json
EOF

  cat > "$pkg/DEBIAN/control" <<EOF
Package: collatech-agent
Version: $VERSION_DEB
Section: utils
Priority: optional
Architecture: $arch
Maintainer: CollaTech <soporte@collatech.local>
Recommends: cups
Description: Agente de impresion ESC/POS para impresoras termicas
 Expone una API REST local que convierte JSON en comandos ESC/POS y los
 manda a impresoras termicas por USB, red, puerto serie o CUPS. Incluye
 panel web, disenador de tickets y SDK para JavaScript.
EOF

  # config.json NO va en el paquete (ni como conffile): la crea postinst
  # si falta. Como conffile, cambiar la de fabrica hacia que dpkg
  # preguntara al actualizar en cada caja cuya config ya habia tocado el
  # panel, y sin terminal la actualizacion fallaba. Asi, la de cada caja
  # no se toca nunca al actualizar.
  cat > "$pkg/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "configure" ]; then
  chmod 750 /var/lib/collatech 2>/dev/null || true
  # Solo en una instalacion nueva. Abierta a la red, como en Windows, y sin
  # token: el agente genera uno al arrancar, asi que desde la red nunca se
  # imprime sin el. Se cierra en el panel: Estado -> Acceso en red.
  if [ ! -e /etc/collatech/config.json ]; then
    mkdir -p /etc/collatech
    cat > /etc/collatech/config.json <<'JSON'
{
  "host": "0.0.0.0",
  "port": 18743,
  "allowed_cors": ["http://localhost:18743"],
  "allow_remote": true,
  "auth_token": "",
  "max_print_size": 2097152,
  "queue": { "workers": 4, "max_retries": 2 }
}
JSON
    chmod 600 /etc/collatech/config.json
  fi
  if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
    # Se comprueba que haya arrancado de verdad antes de decirlo. Antes se
    # anunciaba "en marcha" aunque el enable fallara, y el operador se iba
    # tan tranquilo con el agente parado.
    # La pieza que aplica las actualizaciones del panel. Si falla, el agente
    # funciona igual; solo el boton de actualizar lo dira.
    systemctl enable --now collatech-agent-actualizar.path 2>/dev/null || true
    # restart y no "enable --now": al instalar encima, el agente viejo sigue
    # corriendo en memoria y "--now" no lo toca.
    if systemctl enable collatech-agent.service 2>/dev/null &&
       systemctl restart collatech-agent.service 2>/dev/null &&
       systemctl is-active --quiet collatech-agent.service; then
      echo "CollaTech Agent en marcha: http://localhost:18743/panel"
    else
      echo "CollaTech Agent instalado, pero el servicio NO arranco." >&2
      echo "Mira que paso con:  systemctl status collatech-agent" >&2
    fi
  else
    echo "CollaTech Agent instalado. Sin systemd aqui; arrancalo a mano con:"
    echo "  collatech-agent --config /etc/collatech/config.json --data-dir /var/lib/collatech"
  fi
fi
EOF
  chmod 755 "$pkg/DEBIAN/postinst"

  cat > "$pkg/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "remove" ] && [ -d /run/systemd/system ]; then
  systemctl disable --now collatech-agent-actualizar.path || true
  systemctl disable --now collatech-agent.service || true
fi
EOF
  chmod 755 "$pkg/DEBIAN/prerm"

  cat > "$pkg/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -e
# La config la creo postinst, asi que dpkg no la conoce: purge la borra
# aqui. "remove" la deja, por si se vuelve a instalar.
if [ "$1" = "purge" ]; then
  rm -f /etc/collatech/config.json /etc/collatech/config.json.tmp
  rmdir /etc/collatech 2>/dev/null || true
fi
EOF
  chmod 755 "$pkg/DEBIAN/postrm"

  dpkg-deb --build --root-owner-group "$pkg" \
    "$SALIDA/collatech-agent_${VERSION_DEB}_${arch}.deb" >/dev/null
  rm -rf "$pkg"
  echo "  collatech-agent_${VERSION_DEB}_${arch}.deb"
done

# --- Sumas de verificacion --------------------------------------------------
(cd "$SALIDA" && sha256sum ./*.zip ./*.tar.gz ./*.deb > SHA256SUMS.txt)
echo "  SHA256SUMS.txt"
echo
ls -lh "$SALIDA" | tail -n +2 | awk '{printf "  %-46s %s\n", $9, $5}'
