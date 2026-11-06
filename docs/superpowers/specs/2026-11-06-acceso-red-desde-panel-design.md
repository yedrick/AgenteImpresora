# Acceso desde la red, activado y verificado desde el panel

## Para que

Que cualquier aparato de la red local (celular, tablet, otra PC) pueda
imprimir por el agente, en Windows y en Ubuntu, y que activarlo no exija
editar `config.json` a mano. El dueno quiere ademas **ver que funciona de
verdad**: una prueba desde esta misma PC siempre "pasa" (no cruza el
cortafuegos), asi que la verificacion tiene que venir de otro aparato.

Hoy: el instalador de Windows y `collatech-agent --install` ya dejan la red
abierta (`host 0.0.0.0`, `allow_remote true`, regla de Firewall en Windows).
El `.deb` de Ubuntu la deja cerrada (`127.0.0.1`, `allow_remote false`) y el
diagnostico lo marca como "FALLA bloqueado" sin decir como abrirla.

## Objetivos

1. En la pestana **Estado**, seccion "Acceso en red", un interruptor
   **Acceso desde la red: activado / desactivado**.
2. Al cambiarlo, el agente guarda `host` y `allow_remote` en `config.json`
   y **lo aplica sin reiniciar el proceso**: cierra el puerto y lo vuelve a
   abrir en `0.0.0.0` (o `127.0.0.1` al desactivar). Igual en Windows y
   Linux; no depende de systemd ni del gestor de servicios.
3. Si al activarlo no hay token, se genera y se guarda (lo mismo que ya hace
   el arranque). El panel lo muestra como hoy.
4. **Verificacion desde otro aparato**: el panel muestra la direccion
   `http://<ip-lan>:18743/conectar` en grande y como **QR**. Esa pagina, al
   abrirse desde otro aparato, dice "Conectado a CollaTech Agent" y el
   agente anota la IP y la hora. El panel, que consulta cada pocos
   segundos, pasa a **"Conecto desde 192.168.1.x"**.
5. Si pasado un rato nadie conecta, el panel dice que probar, segun el
   sistema:
   - Ubuntu: si el cortafuegos esta activo, `sudo ufw allow 18743/tcp`
     (para copiar), y revisar que el aparato este en la misma WiFi.
   - Windows: el comando `netsh` de abrir el puerto (para copiar).
   - Los dos: aislamiento de clientes del router (WiFi de invitados).
6. El texto actual "ejecuta ABRIR_FIREWALL_LAN.bat" solo aparece en Windows.

## No objetivos

- El agente **no toca el cortafuegos de Ubuntu** (enfoque B descartado: una
  pieza con permisos de administrador que la pagina pueda disparar es
  demasiada superficie para un agente que abre un cajon de dinero).
- No se abre la administracion a la red.

## Antiobjetivos (si pasa esto, fallo aunque funcione)

- Que desde la red se pueda imprimir sin token.
- Que el interruptor, el token o el registro de conexiones respondan a
  peticiones de la red: son rutas de administracion, solo desde esta PC.
- Que `/conectar` ensene algo util a un intruso: ni token, ni
  configuracion, ni lista de impresoras. Solo "Conectado".
- Que al aplicar el cambio el agente quede sin escuchar en ningun sitio: si
  el puerto nuevo no se puede abrir, se vuelve al anterior y el panel lo
  dice.

## Enfoques considerados

- **A (elegido):** interruptor + verificacion desde otro aparato + comando
  del cortafuegos para copiar. El agente sigue sin tocar el sistema.
- **B:** como A, y el agente abre `ufw` el mismo mediante una unidad root.
  Descartado por seguridad.
- **C:** `.deb` abierto a la red de fabrica. Al principio descartado; tras
  revisar el diseno el dueno pidio **ademas** de A que la red venga abierta
  por defecto siempre, igual que en Windows. Queda A + C: de fabrica
  abierto (con token obligatorio desde la red, generado al primer
  arranque) y el interruptor para cerrarlo o volver a abrirlo.

## Decisiones tomadas

- **De fabrica, abierto a la red** en todos los paquetes (`.deb` y la
  `config.json` de partida de los zip/tar): `host "0.0.0.0"`,
  `allow_remote true`, `auth_token ""` (el agente lo genera y lo guarda al
  arrancar). Las instalaciones que ya existen conservan su configuracion
  (desde esta version `config.json` no va en el `.deb`: `postinst` la crea solo si falta, asi que actualizar nunca la toca ni hace preguntar a dpkg); para esas esta el interruptor.
  `config.Default()` (sin archivo de configuracion) sigue en solo esta PC.

- Se aplica en caliente reabriendo el puerto, no cerrando el proceso: en
  Windows el servicio no se reinicia solo al salir.
- El filtro "solo esta PC" (`localhostOnly`) deja de decidirse al montar
  las rutas: consulta el estado vivo de `allow_remote`.
- `/conectar` no pide token (desde la barra de direcciones del celular no
  se pueden mandar cabeceras) y solo responde si el acceso por red esta
  activo. Guarda en memoria las ultimas conexiones (IP y hora), no en disco.
- Las rutas nuevas de administracion (interruptor, conexiones, QR) van en
  `adminPaths`.

## Lo que decide quien lo construya

- Como se reabre el listener (nuevo `http.Server` sobre el mismo handler,
  conservando TLS si esta activo).
- Si el QR se sirve como PNG desde el agente (ya esta `rsc.io/qr`) o se
  dibuja en el panel.
- Cada cuanto consulta el panel y cuanto espera antes de mostrar los
  consejos del punto 5.

## Como se verifica

- Pruebas en Go: interruptor guarda y aplica; token generado; rutas admin
  rechazan la red; `/conectar` responde solo con la red activa y anota la
  IP; si el puerto nuevo falla, se queda el anterior.
- En este Ubuntu real: activar desde el panel y abrir `/conectar` desde un
  contenedor Docker por la IP de la WiFi (cruza la interfaz de red, no
  `localhost`); el panel tiene que marcar la conexion. Desactivar y ver
  que desde ahi ya no responde.
- Windows: lo prueba el dueno.
