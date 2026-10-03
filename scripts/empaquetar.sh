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

cd "$RAIZ"
rm -rf "$SALIDA"
mkdir -p "$SALIDA"

# --- Binarios ---------------------------------------------------------------
compilar() {
  local os="$1" arch="$2" nombre="$3" paquete="${4:-./cmd/server}"
  echo "  $nombre"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 \
    go build -trimpath -ldflags "$LDFLAGS" -o "$SALIDA/$nombre" "$paquete"
}

echo "Compilando $VERSION:"
compilar linux   amd64 collatech-agent-linux-amd64
compilar linux   arm64 collatech-agent-linux-arm64
compilar darwin  amd64 collatech-agent-macos-intel
compilar darwin  arm64 collatech-agent-macos-apple
compilar windows amd64 CollaTechAgent-windows-amd64.exe
compilar windows 386   CollaTechAgent-windows-386.exe
compilar windows amd64 INSTALADOR-windows-amd64.exe ./cmd/installer

# --- Lo que acompana a cada paquete -----------------------------------------
comunes() {
  local destino="$1"
  mkdir -p "$destino/configs" "$destino/docs"
  cp README.md JSON-REFERENCIA.md ANGULAR.md "$destino/docs/"
  cp docs/API.md "$destino/docs/" 2>/dev/null || true
  # Config de partida sin token: el agente genera uno al arrancar.
  cat > "$destino/configs/config.json" <<'EOF'
{
  "host": "127.0.0.1",
  "port": 18743,
  "allowed_cors": ["http://localhost:18743"],
  "allow_remote": false,
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
  mkdir -p "$pkg/DEBIAN" "$pkg/usr/local/bin" "$pkg/etc/collatech" \
           "$pkg/var/lib/collatech" "$pkg/lib/systemd/system" \
           "$pkg/usr/share/doc/collatech-agent"
  cp "$SALIDA/$binario" "$pkg/usr/local/bin/collatech-agent"
  chmod 755 "$pkg/usr/local/bin/collatech-agent"
  cp README.md JSON-REFERENCIA.md "$pkg/usr/share/doc/collatech-agent/"

  cat > "$pkg/etc/collatech/config.json" <<'EOF'
{
  "host": "127.0.0.1",
  "port": 18743,
  "allowed_cors": ["http://localhost:18743"],
  "allow_remote": false,
  "auth_token": "",
  "max_print_size": 2097152,
  "queue": { "workers": 4, "max_retries": 2 }
}
EOF

  cat > "$pkg/lib/systemd/system/collatech-agent.service" <<'EOF'
[Unit]
Description=CollaTech Agent - impresion ESC/POS
After=network-online.target cups.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/collatech-agent --config /etc/collatech/config.json --data-dir /var/lib/collatech
Restart=on-failure
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

  cat > "$pkg/DEBIAN/conffiles" <<'EOF'
/etc/collatech/config.json
EOF

  cat > "$pkg/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
if [ "$1" = "configure" ]; then
  chmod 750 /var/lib/collatech 2>/dev/null || true
  if [ -d /run/systemd/system ]; then
    systemctl daemon-reload || true
    # Se comprueba que haya arrancado de verdad antes de decirlo. Antes se
    # anunciaba "en marcha" aunque el enable fallara, y el operador se iba
    # tan tranquilo con el agente parado.
    if systemctl enable --now collatech-agent.service 2>/dev/null &&
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
  systemctl disable --now collatech-agent.service || true
fi
EOF
  chmod 755 "$pkg/DEBIAN/prerm"

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
