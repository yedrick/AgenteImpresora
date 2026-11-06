# Actualizar el agente desde el panel

Desde la versión 1.6.4, el agente se actualiza con un botón:

1. En la PC del agente, abre **http://localhost:18743/panel**.
2. En la pestaña **Estado**, tarjeta **Actualizaciones**, pulsa **Buscar actualización**.
3. Si hay una versión nueva, pulsa **Instalar**.

El agente hace lo siguiente:

1. Descarga la versión nueva de GitHub y **comprueba que está firmada** por ti.
2. Se reinicia con ella. Durante unos segundos no imprime.
3. Comprueba que la versión nueva responde. **Si no responde, vuelve sola a la anterior**, con su configuración.

Se conservan el token, las webs autorizadas, la conexión con la nube y las impresoras dadas de alta.

Funciona en Windows (servicio de Windows) y en Linux con systemd (el `.deb` o `INSTALAR.sh`). En Mac se actualiza con el paquete a mano.

**Por seguridad:**

- El botón solo funciona en la propia PC del agente, no desde otra PC de la red.
- El agente nunca busca actualizaciones por su cuenta: solo cuando alguien pulsa el botón.

## La primera vez: pasar a la 1.6.4 a mano

Las versiones anteriores no tienen el botón, así que el salto a la 1.6.4 se hace a mano una sola vez:

- **Windows:** descarga `CollaTechAgent-1.6.4-windows.zip`, descomprímelo y ejecuta `INSTALADOR.exe`. Detecta la instalación anterior y conserva la configuración.
- **Ubuntu / Debian:** `sudo dpkg -i collatech-agent_1.6.4_amd64.deb`.
- **Otro Linux (tar.gz):** `sudo ./INSTALAR.sh` dentro del paquete nuevo.

En Linux, el paquete instala dos piezas pequeñas de systemd que son las que hacen el cambio. Sin ellas, el panel avisa de que hay que instalar el paquete a mano:

- `collatech-agent-actualizar.path`
- `collatech-agent-actualizar.service`

## Las claves de firma (para quien publica)

Cada versión que publica GitHub va firmada. El agente solo instala lo que lleve una firma válida de una de estas dos claves:

| Clave | Dónde vive la privada | Para qué |
|---|---|---|
| **Principal** | En el secreto `FIRMA_ACTUALIZACIONES` de GitHub y en tu copia | Firma cada publicación |
| **Respaldo** | **Solo** en tu copia, fuera de internet | Por si la principal se pierde o se filtra |

Las dos se generaron en `~/.collatech-firma/principal/` y `~/.collatech-firma/respaldo/`. Cada carpeta tiene un `privada.txt` y un `publica.txt`. Las públicas ya están escritas en `internal/actualizar/claves.go`.

### Configurar el secreto en GitHub (una vez, 5 minutos)

1. Abre el contenido de la clave privada principal:

   ```bash
   cat ~/.collatech-firma/principal/privada.txt
   ```

2. En GitHub, ve a **yedrick/AgenteImpresora → Settings → Secrets and variables → Actions → New repository secret**.
3. En **Name** escribe `FIRMA_ACTUALIZACIONES`.
4. En **Secret** pega el contenido del paso 1, tal cual.
5. Pulsa **Add secret**.

Sin este secreto, la publicación falla a propósito en el paso "Firmar el manifiesto de actualizacion". Así nunca sale una versión que el botón no podría instalar.

### Guardar las claves

- Copia la carpeta `~/.collatech-firma` entera a un **USB o un gestor de contraseñas**. Si se pierde la principal y la de respaldo, las cajas solo podrán actualizarse a mano.
- **Nunca** subas `privada.txt` a ningún repositorio ni la mandes por chat. `cmd/firmar` se niega a crear claves dentro de un repositorio, y `.gitignore` ignora `privada.txt`.
- La de respaldo **no** va a GitHub.

### Si la clave principal se filtra o se pierde

1. Genera una principal nueva, fuera del repositorio:

   ```bash
   go run ./cmd/firmar generar ~/.collatech-firma/principal-2
   ```

2. Pon su `publica.txt` en `internal/actualizar/claves.go` **en lugar de** la principal vieja. Deja la de respaldo como está.
3. Cambia el secreto `FIRMA_ACTUALIZACIONES` por la **privada de respaldo** y publica esa versión. Las cajas la aceptan por la firma de respaldo, y a partir de ahí confían en la principal nueva.
4. Vuelve a poner en el secreto la **privada principal nueva** para las siguientes versiones.

## Frenar el botón en una versión concreta

Si una versión necesita algo más que cambiar el programa (cambios en las unidades de systemd, en el firewall o en los certificados), se publica con un aviso. En `scripts/empaquetar.sh`, la línea del manifiesto admite:

- `-manual "texto"`: el panel explica el texto y no ofrece instalar. Esa versión se instala a mano.
- `-minima X`: desde versiones menores que X, el panel pide instalar a mano.

## Si algo falla

- **"No se pudo consultar GitHub"**: la PC no tiene internet, o GitHub limitó las consultas (60 por hora por red). Espera y vuelve a probar.
- **"La firma del manifiesto no es válida"**: no instales nada. O la descarga no viene de las publicaciones oficiales, o se publicó con una clave que no es la tuya.
- **"La versión X no respondió; se volvió a la Y"**: el agente sigue funcionando con la versión anterior. Mira los registros:
  - Windows: el panel, pestaña Registros.
  - Linux: `journalctl -u collatech-agent -u collatech-agent-actualizar`.
