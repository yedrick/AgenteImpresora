# Acceso desde la red desde el panel — plan de implementacion

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Un interruptor en el panel abre o cierra el agente a la red local sin reiniciarlo, y una pagina `/conectar` abierta desde otro aparato demuestra que de verdad llega.

**Architecture:** Un paquete nuevo `internal/oyente` cambia el socket de escucha en caliente (cierra el viejo, abre el nuevo, vuelve atras si falla) sobre el mismo `http.Server`. `api.Server` guarda el estado de red vivo (host, allow_remote, token) tras un mutex; el middleware lo consulta en cada peticion en vez de decidirlo al montar las rutas. El panel usa `GET/POST /api/red` (solo esta PC) y `GET /api/red/qr.png`; la verificacion es `GET /conectar`, publica, que anota la IP de quien la abre.

**Tech Stack:** Go 1.22 (net/http, rsc.io/qr ya en go.mod), panel HTML/JS embebido en `internal/api/web/index.html`.

**Spec:** `docs/superpowers/specs/2026-11-06-acceso-red-desde-panel-design.md`

## Global Constraints

- Desde la red **siempre** hace falta token para `/api/*`; nada de este plan lo relaja.
- `/api/red` y `/api/red/qr.png` van en `adminPaths`: solo desde la propia PC.
- `/conectar` no ensena token, configuracion ni impresoras: solo "Conectado".
- Si el puerto nuevo no se puede abrir, el agente vuelve a escuchar donde estaba y la respuesta lo dice; `config.json` no cambia.
- Se aplica sin reiniciar el proceso (Windows no relanza el servicio al salir).
- Activar = `host "0.0.0.0"`, `allow_remote true`. Desactivar = `host "127.0.0.1"`, `allow_remote false`. El token no se borra al desactivar.
- Comando de cortafuegos que el panel ofrece: Linux `sudo ufw allow 18743/tcp`; Windows `netsh advfirewall firewall add rule name="CollaTech Agent 18743" dir=in action=allow protocol=TCP localport=18743` (el puerto real, no fijo).
- Textos en espanol sin tildes en el codigo Go (estilo del repo); el panel puede llevarlas.
- Sin atribucion a Claude/Anthropic en commits ni documentos.

## Review Focus

1. La propia peticion `POST /api/red` viaja por el socket que se cierra: tiene que recibir su respuesta (cerrar el listener no corta conexiones ya aceptadas). Test en Task 1.
2. `/conectar` abierto desde esta misma PC no puede contar como "conecto desde la red". Test en Task 3.
3. Con la red desactivada, una peticion de la red a cualquier ruta sigue recibiendo 403 aunque el filtro ya no se decida al montar. Test en Task 2.
4. Una `config.json` que no se puede escribir: el interruptor falla con mensaje y no deja el socket cambiado. Test en Task 2 (guardar falla → vuelve atras).
5. Muchas visitas a `/conectar` no hacen crecer la memoria: solo las ultimas 10. Test en Task 3.

---

### Task 1: `internal/oyente` — cambiar el socket en caliente

**Files:**
- Create: `internal/oyente/oyente.go`
- Test: `internal/oyente/oyente_test.go`

**Interfaces:**
- Produces:
  - `type Oyente struct` (mutex, `*http.Server`, listener actual, host actual, puerto, `*tls.Config`)
  - `func Abrir(srv *http.Server, host string, puerto int, tlsCfg *tls.Config) (*Oyente, error)` — escucha y lanza `srv.Serve` en una goroutine.
  - `func (o *Oyente) Reabrir(host string) error` — mismo host: no hace nada. Si no: cierra solo el listener (no `Shutdown`), escucha en `host:puerto`; si falla, vuelve a escuchar en el host anterior y devuelve el error original envuelto ("no se pudo escuchar en X: ...; sigue en Y").
  - `func (o *Oyente) Host() string`

- [ ] **Step 1: Tests que fallan**

```go
func TestReabrirCambiaDondeEscucha(t *testing.T)
// Abrir en 127.0.0.1:<libre>; GET /health por 127.0.0.1 → 200.
// Reabrir("0.0.0.0"); GET por 127.0.0.1 sigue 200 y Host()=="0.0.0.0".
// Reabrir("127.0.0.1"); Host()=="127.0.0.1".

func TestReabrirQueFallaVuelveAlAnterior(t *testing.T)
// Reabrir("192.0.2.1") (TEST-NET, no es de esta maquina) → error que
// contiene "sigue en 127.0.0.1"; Host()=="127.0.0.1"; GET por 127.0.0.1 → 200.

func TestLaPeticionQueReabreRecibeSuRespuesta(t *testing.T)
// Un handler en /cambiar llama o.Reabrir("0.0.0.0") y escribe "ok".
// GET /cambiar por 127.0.0.1 → 200 y cuerpo "ok".
```
Puerto libre: `net.Listen("tcp","127.0.0.1:0")`, leer el puerto, cerrar.

- [ ] **Step 2:** `go test ./internal/oyente/` → FAIL (no compila: `Abrir` no existe).
- [ ] **Step 3:** Implementar en `internal/oyente/oyente.go`. Con TLS, envolver con `tls.NewListener`. `http.Server.Serve` admite volver a llamarse tras cerrar solo el listener.
- [ ] **Step 4:** `go test ./internal/oyente/ -race -count=1` → PASS.
- [ ] **Step 5:** Commit `Oyente: cambiar el socket de escucha en caliente`.

### Task 2: estado de red vivo y `GET/POST /api/red`

**Files:**
- Create: `internal/api/handlers_red.go`
- Modify: `internal/api/server.go` (campos, rutas, `localhostOnly` siempre montado), `internal/api/middleware.go` (`adminPaths`, guard y localhostOnly leen el estado vivo), `internal/api/handlers_admin.go` (`network`, `readToken` leen el estado vivo), `internal/api/diagnostics.go` (host/allow_remote vivos)
- Test: `internal/api/red_test.go`

**Interfaces:**
- Consumes: nada de Task 1 directamente (se inyecta como funcion).
- Produces:
  - En `Server`: `redMu sync.RWMutex`; `Reabrir func(host string) error` (exportado; lo pone main; nil = "aqui no se puede cambiar").
  - `func (s *Server) red() (host string, remoto bool, token string)`
  - `GET /api/red` → `{"activo","host","puerto","token_requerido","urls_prueba":[ "http://<ip>:<puerto>/conectar" ],"conexiones":[{"ip","hora"}],"cortafuegos":"<comando>","sistema":runtime.GOOS}`
  - `POST /api/red` con `{"activo":bool}` → 200 con el mismo cuerpo que GET; 500 con `error` si Reabrir o guardar fallan.
  - `func comandoCortafuegos(goos string, puerto int) string` (lo reutiliza `diagnostics.go` para `como_abrir`).
- Orden en POST: leer `config.json` (como `saveCORS`) → token nuevo con `config.NewToken()` si activa y no hay → `Reabrir(host nuevo)` → `config.Save`; si guardar falla, `Reabrir(host viejo)` y 500 → actualizar estado vivo.

- [ ] **Step 1: Tests que fallan** (helper nuevo `newTestServerAPI(t, tune) *Server`, mismo montaje que `newTestServer`; `Reabrir` falso que anota el host pedido)

```go
func TestActivarRedGuardaYAplica(t *testing.T)
// POST /api/red {"activo":true} desde 127.0.0.1 → 200; Reabrir recibio "0.0.0.0";
// config.json tiene host "0.0.0.0", allow_remote true y auth_token no vacio;
// GET /api/status desde 192.168.1.77 sin token → 401; con ese token → 200.

func TestDesactivarRedVuelveASoloEstaPC(t *testing.T)
// Con red activa, POST {"activo":false} → Reabrir("127.0.0.1"); config allow_remote false,
// auth_token se conserva; GET /health desde 192.168.1.77 → 403.

func TestRedNoSeCambiaDesdeLaRed(t *testing.T)
// POST /api/red y GET /api/red desde 192.168.1.77 con token valido → 403.

func TestRedSiReabrirFallaNoGuarda(t *testing.T)
// Reabrir devuelve error → 500, error contiene el mensaje; config.json sin cambios; GET /api/red activo=false.

func TestRedSiGuardarFallaVuelveAtras(t *testing.T)
// paths.Config apunta a un directorio de solo lectura (chmod 0500; t.Skip si corre como root)
// → 500; Reabrir llamado con "0.0.0.0" y luego "127.0.0.1"; activo=false.

func TestComandoCortafuegos(t *testing.T)
// linux,18743 → "sudo ufw allow 18743/tcp"; windows,18743 → contiene "localport=18743" y "netsh".
```

- [ ] **Step 2:** `go test ./internal/api -run 'Red|Cortafuegos'` → FAIL (no compila).
- [ ] **Step 3:** Implementar. `Routes()` monta `localhostOnly` siempre; dentro, deja pasar si `red()` dice remoto. `guard` usa el token de `red()`.
- [ ] **Step 4:** `go test ./internal/api -race -count=1` → PASS (las pruebas existentes de allow_remote siguen verdes).
- [ ] **Step 5:** Commit `Panel: activar el acceso desde la red sin reiniciar`.

### Task 3: `/conectar`, registro de conexiones y QR

**Files:**
- Modify: `internal/api/handlers_red.go`, `internal/api/server.go` (rutas), `internal/api/middleware.go` (`/api/red/qr.png` en adminPaths)
- Test: `internal/api/red_test.go`

**Interfaces:**
- Produces:
  - `type Conexion struct { IP string `json:"ip"`; Hora time.Time `json:"hora"` }`; `conexiones []Conexion` en Server (bajo `redMu`), maximo 10, la mas nueva primero.
  - `GET /conectar` (fuera de `/api/`, sin token): red inactiva → 404 texto "El acceso desde la red esta desactivado". Activa y peticion local → 200 HTML que dice "Esta es la misma PC" y **no** anota. Activa y remota → 200 HTML "Conectado a CollaTech Agent" y anota IP y hora.
  - `GET /api/red/qr.png` → PNG del primer `urls_prueba` (`qr.Encode(url, qr.M).PNG()`); 404 si no hay IP de red.

- [ ] **Step 1: Tests que fallan**

```go
func TestConectarAnotaLaIPDeOtroAparato(t *testing.T)
// red activa; GET /conectar desde 192.168.1.77:5000 → 200, cuerpo contiene "Conectado";
// GET /api/red local → conexiones[0].ip == "192.168.1.77".

func TestConectarDesdeEstaPCNoCuenta(t *testing.T)
// red activa; GET /conectar desde 127.0.0.1 → 200, contiene "misma PC"; conexiones vacio.

func TestConectarConRedApagada(t *testing.T)
// red inactiva; GET /conectar desde 127.0.0.1 → 404.

func TestConectarNoEnsenaNadaUtil(t *testing.T)
// cuerpo de /conectar remoto no contiene el token ni "auth_token" ni "printer".

func TestConexionesSeQuedanEnDiez(t *testing.T)
// 25 GET /conectar desde 192.168.1.1..25 → len(conexiones)==10, la primera es .25.

func TestQRDeLaRedEsSoloLocal(t *testing.T)
// GET /api/red/qr.png desde 192.168.1.77 con token → 403.
```

- [ ] **Step 2:** `go test ./internal/api -run 'Conectar|Conexiones|QR'` → FAIL.
- [ ] **Step 3:** Implementar. La IP sale de `net.SplitHostPort(r.RemoteAddr)`.
- [ ] **Step 4:** `go test ./internal/api -race -count=1` → PASS.
- [ ] **Step 5:** Commit `Red: pagina /conectar para comprobar desde otro aparato`.

### Task 4: conectar main con el oyente

**Files:**
- Modify: `cmd/server/main.go` (`startServer`, `agent`)
- Test: `cmd/server/main_test.go` (o el existente)

**Interfaces:**
- Consumes: `oyente.Abrir`, `(*Oyente).Reabrir`; `api.Server.Reabrir`.
- `startServer` sustituye su `net.Listen` + `tls.NewListener` + `go server.Serve` por `oyente.Abrir(server, cfg.Host, cfg.Port, tlsCfg)` y pone `srv.Reabrir = o.Reabrir` antes de servir. Errores de arranque iguales que hoy ("no se pudo escuchar en ...").

- [ ] **Step 1: Test que falla** `TestArrancarYActivarRed`: `startServer` con config en un temp (host 127.0.0.1, puerto libre, data dir temp); POST `/api/red {"activo":true}` por `http://127.0.0.1:<p>` → 200; GET `/health` por 127.0.0.1 → 200; `config.Load` dice host 0.0.0.0; `agent.shutdown()`.
- [ ] **Step 2:** `go test ./cmd/server -run TestArrancarYActivarRed` → FAIL (Reabrir nil → 500).
- [ ] **Step 3:** Implementar.
- [ ] **Step 4:** `go test ./... -race -count=1` → PASS.
- [ ] **Step 5:** Commit `Agente: el interruptor de red cambia el socket de verdad`.

### Task 5: panel, documentacion y prueba real

**Files:**
- Modify: `internal/api/web/index.html` (seccion "Acceso en red"), `internal/api/web/diagnostico.html` (texto de "Acceso LAN" bloqueado: decir "activalo en el panel, Estado → Acceso en red"), `docs/UBUNTU.md` (seccion corta "Imprimir desde otros aparatos")
- Test: `internal/api/web_test.go`

**Interfaces:**
- Consumes: `GET/POST /api/red`, `GET /api/red/qr.png`.
- Elementos: `#redActivo` (checkbox con etiqueta "Acceso desde la red"), `#redEstado` (aria-live), `#redPrueba` (URL en grande + `<img id="redQR">`), `#redConsejos` (oculto hasta 60 s sin conexion; muestra `cortafuegos` en `<code>` con boton copiar y la nota de WiFi de invitados). Consulta `/api/red` cada 3 s solo mientras esta activo y la pestana Estado esta visible. La linea "ABRIR_FIREWALL_LAN.bat" solo si `sistema == "windows"`.

- [ ] **Step 1: Test que falla** `TestPanelTieneInterruptorDeRed`: el HTML servido contiene `id="redActivo"`, `id="redQR"`, `/api/red` y la llamada `cargarRed()` a nivel superior (misma comprobacion de sangria que `TestElPanelConectaSusBotonesAlCargar`).
- [ ] **Step 2:** `go test ./internal/api -run TestPanelTieneInterruptorDeRed` → FAIL.
- [ ] **Step 3:** Implementar el panel y la doc.
- [ ] **Step 4:** `go test ./... -race -count=1` → PASS; `make version-check`; `./scripts/comprobar-ejemplos.sh`.
- [ ] **Step 5: Prueba real en este Ubuntu** (binario de la rama corriendo a mano en un puerto libre, p. ej. 18799, con config y datos en el scratchpad, no tocar el agente instalado):
  1. `playwright-cli` abre el panel, marca `#redActivo`; `#redEstado` dice activado.
  2. `docker run --rm curlimages/curl -s http://<ip-wifi>:18799/conectar` → contiene "Conectado".
  3. El panel muestra "Conecto desde 172.17.x.x" (o la IP que vea el agente).
  4. Desmarcar; el mismo `docker run` ya no conecta.
  5. Medir RSS antes y despues (memoria del dueno: medir RAM).
- [ ] **Step 6:** Commit `Panel: interruptor de red con prueba desde otro aparato`.

### Task 6: de fabrica, abierto a la red

**Files:**
- Modify: `scripts/empaquetar.sh` (las dos `config.json` de partida: `comunes()` y la del `.deb`)
- Modify: `.github/workflows/ci.yml` (paso "Comprobar que el .deb es valido e instalable")
- Modify: `docs/UBUNTU.md`, `docs/superpowers/specs/2026-11-06-acceso-red-desde-panel-design.md` (ya dice A + C)

**Interfaces:** ninguna nueva. `host "0.0.0.0"`, `allow_remote true`, `auth_token ""`.

- [ ] **Step 1: Test que falla (CI)** tras `dpkg --install` en el job de paquetes:
  ```bash
  ip=$(hostname -I | awk '{print $1}')
  curl -sf "http://$ip:18743/health"                       # llega por la red
  [ "$(curl -s -o /dev/null -w '%{http_code}' "http://$ip:18743/api/status")" = 401 ]  # sin token, no
  sudo grep -q '"auth_token": "[^"]' /etc/collatech/config.json   # token generado
  ```
- [ ] **Step 2:** push → el job "Paquetes de instalacion" FALLA (hoy escucha solo en 127.0.0.1).
- [ ] **Step 3:** Cambiar las dos `config.json` de `empaquetar.sh`.
- [ ] **Step 4:** push → el job PASA.
- [ ] **Step 5:** Commit `Paquetes: abiertos a la red de fabrica, con token obligatorio`.
