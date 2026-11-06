# Actualizar el agente desde el panel - Plan de implementacion

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Boton "Buscar actualizacion" en el panel que descarga, verifica la firma e instala la version nueva del agente en Windows y Linux, volviendo sola a la anterior si la nueva no responde.

**Architecture:** Un paquete nuevo `internal/actualizar` hace todo lo que no depende del sistema:
- comparar versiones;
- verificar el manifiesto firmado (Ed25519);
- consultar y descargar de GitHub;
- orquestar la busqueda y la instalacion;
- aplicar el cambio con vuelta atras.

Lo que depende del sistema (parar y arrancar el servicio, lanzar el aplicador) vive detras de una interfaz `Sistema` con una implementacion para Windows y otra para Linux. El ejecutable gana el modo `--aplicar-actualizacion <orden.json>`:
- en Windows lo lanza el servicio, desde una copia del ejecutable actual;
- en Linux lo lanza una unidad systemd `.path` que vigila la orden.

El workflow de Release publica los ejecutables sueltos, un manifiesto y su firma.

**Tech Stack:**
- Go 1.22: `crypto/ed25519` y `golang.org/x/sys/windows/svc/mgr`, que ya esta en `go.mod`.
- systemd: unidades `.path` y `.service`.
- GitHub Actions.
- JavaScript sin framework en el panel.

**Spec:** `docs/superpowers/specs/2026-11-05-actualizar-desde-panel-design.md`

## Global Constraints

- Repositorio de publicaciones: `yedrick/AgenteImpresora`. API: `https://api.github.com/repos/yedrick/AgenteImpresora/releases/latest`.
- El agente solo consulta GitHub cuando se pulsa el boton: nada en segundo plano.
- Firma Ed25519 sobre los bytes exactos de `manifiesto.json`. Se acepta la firma de cualquiera de las dos claves oficiales (principal y respaldo).
- Secreto de GitHub Actions: `FIRMA_ACTUALIZACIONES` (clave privada en base64). Si falta, la Release falla.
- Comprobacion tras actualizar: hasta 60 s a que `/health` responda con la version nueva. Espera de la cola antes de reiniciar: hasta 30 s.
- Se conservan `config.json`, `storage/settings.json` y `storage/jobs.json`. Se copian antes del cambio y se restauran si se vuelve atras.
- `/api/actualizacion*` solo desde la PC del agente (`adminPaths` y `adminWritePaths`).
- Solo se ofrecen versiones mayores (semver). macOS: no soportado, con un mensaje.
- User-Agent de las peticiones: `CollaTech-Agent/<version>`. Sin datos del usuario.
- Textos en espanol sin tildes en el codigo Go, como el resto del repo. El panel si las lleva donde ya las lleva.
- Nombres de los recursos de la Release:
  - `actualizacion-linux-amd64`
  - `actualizacion-linux-arm64`
  - `actualizacion-windows-amd64.exe`
  - `actualizacion-windows-386.exe`
  - `manifiesto.json`
  - `manifiesto.json.sig`

  Los de Windows se compilan con `-H windowsgui`, como el que mete el instalador.

## Review Focus

1. **Sin internet, o GitHub responde 403 por limite de consultas.** El panel tiene que decir "No se pudo consultar GitHub: ..." y no quedarse en "Buscando...". Tarea 3: prueba `TestUltimaErrorHTTP`.
2. **Descarga cortada o archivo cambiado.** El SHA256 no coincide: no se toca nada y el error lo dice. Tarea 2: `TestVerificarArchivoNoCoincide`. Tarea 4: `TestInstalarArchivoCorrupto`.
3. **La version nueva arranca pero no responde, o responde otra version.** Vuelve atras: se restauran el ejecutable y los datos, y queda `resultado.json` con el fallo. Tarea 5: `TestAplicarVuelveAtrasSiNoResponde` y `TestAplicarVuelveAtrasSiVersionDistinta`.
4. **Pulsar Instalar dos veces, o desde dos pestanas.** Solo una a la vez; la segunda recibe un error claro. Tarea 4: `TestInstalarSoloUnaVez`.
5. **Orden manipulada en la carpeta de datos** (apunta a otro ejecutable u otro manifiesto). El aplicador vuelve a verificar la firma y el SHA256, y rechaza la orden. Tarea 5: `TestAplicarRechazaOrdenManipulada`.

---

## Archivos

| Archivo | Responsabilidad |
|---|---|
| `internal/actualizar/version.go` | `Comparar(a, b string) int`, `EsMayor` |
| `internal/actualizar/firma.go` | `Manifiesto`, `VerificarManifiesto`, `VerificarArchivo`, claves oficiales |
| `internal/actualizar/claves.go` | las dos claves publicas oficiales (base64) |
| `internal/actualizar/github.go` | `Fuente`: ultima Release y descargas |
| `internal/actualizar/actualizador.go` | `Actualizador`: estado, `Buscar`, `Instalar` (asincrono) |
| `internal/actualizar/aplicar.go` | `Orden`, `Aplicar(orden, sistema)`: cambio, comprobacion y vuelta atras |
| `internal/actualizar/sistema.go` | interfaz `Sistema` y `Plataforma()` |
| `internal/actualizar/sistema_windows.go` | servicio por SCM, ruta real del exe, lanzar el aplicador desacoplado |
| `internal/actualizar/sistema_linux.go` | `systemctl`, lanzar = escribir la orden (la recoge la unidad `.path`) |
| `internal/actualizar/sistema_otros.go` | macOS y otros: no soportado |
| `cmd/firmar/main.go` | `generar` claves, `manifiesto` y `firmar` (lo usa el workflow) |
| `cmd/server/main.go` | flag `--aplicar-actualizacion`; conecta el `Actualizador` al `Server` |
| `cmd/server/service_unix.go` | `--install` escribe tambien las unidades de actualizar |
| `internal/api/handlers_actualizar.go` | `GET /api/actualizacion`, `POST .../buscar`, `POST .../instalar` |
| `internal/api/handlers_pages.go` | `/health` incluye `data.version` |
| `internal/api/middleware.go` | las rutas nuevas, solo en local |
| `internal/api/web/index.html` | tarjeta "Actualizaciones" en la pestana Estado |
| `scripts/empaquetar.sh` | ejecutables `actualizacion-*`, `manifiesto.json`, unidades en el `.deb` |
| `.github/workflows/release.yml` | firmar, fallar sin secreto, subir los recursos nuevos |
| `.github/workflows/ci.yml` | prueba de punta a punta en Linux con systemd |
| `docs/ACTUALIZAR.md` | guia: configurar la clave, como se actualiza, que hacer si falla |

---

### Task 1: Versiones y firma

**Files:**
- Create: `internal/actualizar/version.go`, `internal/actualizar/firma.go`, `internal/actualizar/claves.go`
- Test: `internal/actualizar/version_test.go`, `internal/actualizar/firma_test.go`

**Interfaces:**
- Produce:
  - `func Comparar(a, b string) int`: -1, 0 o 1. Acepta `v1.6.3` y `1.6.3`. Una version no numerica (`dev`, `ci-abc`) es menor que cualquier numerica.
  - `func EsMayor(nueva, actual string) bool`
  - `type Archivo struct { Nombre string `json:"nombre"`; SHA256 string `json:"sha256"` }`
  - `type Manifiesto struct { Version string `json:"version"`; Minima string `json:"minima,omitempty"`; Manual string `json:"manual,omitempty"`; Archivos map[string]Archivo `json:"archivos"` }`. La clave de `Archivos` es la `Plataforma()`, por ejemplo `linux-amd64`.
  - `func VerificarManifiesto(datos, firma []byte, claves []ed25519.PublicKey) (Manifiesto, error)`: `firma` es la firma en base64 (con espacios o saltos alrededor). Ignora campos desconocidos.
  - `func VerificarArchivo(ruta, sha256hex string) error`
  - `func ClavesOficiales() []ed25519.PublicKey`: decodifica `clavesOficiales` de `claves.go`.

- [ ] **Step 1: Tests que fallan.**
  - `TestComparar`, en tabla:
    - `("1.6.3","1.6.10")=-1`
    - `("v1.7.0","1.6.9")=1`
    - `("1.6.3","1.6.3")=0`
    - `("dev","0.0.1")=-1`
  - `TestManifiestoFirmaValida`: firma con una clave generada en la prueba y verifica con `[]ed25519.PublicKey{pub}`.
  - `TestManifiestoFirmaDeRespaldo`: firma con la segunda clave y verifica con `{pub1, pub2}`.
  - `TestManifiestoFirmaInvalida`: un byte cambiado en los datos da error.
  - `TestManifiestoCampoDesconocido`: un JSON con `"nuevo":1` se acepta.
  - `TestVerificarArchivoNoCoincide`: el error contiene `"no coincide"`.
- [ ] **Step 2:** `go test ./internal/actualizar/` falla porque no compila.
- [ ] **Step 3: Implementar.**
  - `claves.go` lleva `var clavesOficiales = []string{"<principal>", "<respaldo>"}`, en base64 de 32 bytes. Las da la Tarea 2.
  - Hasta entonces, un marcador que `ClavesOficiales()` descarta si no decodifica a 32 bytes.
- [ ] **Step 4:** `go test ./internal/actualizar/` pasa.
- [ ] **Step 5:** Commit "Actualizar: versiones y firma del manifiesto".

### Task 2: Herramienta de firma, claves oficiales y publicacion

**Files:**
- Create: `cmd/firmar/main.go`, `docs/ACTUALIZAR.md`
- Modify: `internal/actualizar/claves.go`, `scripts/empaquetar.sh`, `.github/workflows/release.yml`, `.gitignore`
- Test: `cmd/firmar/main_test.go`

**Interfaces:**
- Consume: `Manifiesto`, `Archivo`, `VerificarManifiesto` (Tarea 1).
- Produce:
  - `firmar generar <carpeta>`: escribe `privada.txt` (seed de 32 bytes en base64, permisos 0600) y `publica.txt`. Se niega a escribir dentro del repositorio.
  - `firmar manifiesto -dir dist -version X [-minima Y] [-manual "texto"]`: lee los `actualizacion-*` de `dist` y escribe `dist/manifiesto.json`.
  - `firmar firmar <archivo>`: lee el seed de la variable `FIRMA_ACTUALIZACIONES`, escribe `<archivo>.sig` y verifica la firma antes de terminar. Si la variable falta, sale con error: `"falta el secreto FIRMA_ACTUALIZACIONES: ver docs/ACTUALIZAR.md"`.

- [ ] **Step 1: Tests.**
  - `TestManifiestoDesdeCarpeta`: dos archivos falsos `actualizacion-linux-amd64` y `actualizacion-windows-386.exe` dan las claves `linux-amd64` y `windows-386` con sus SHA256.
  - `TestFirmarSinSecreto`: devuelve un error que contiene `FIRMA_ACTUALIZACIONES`.
  - `TestFirmarYVerificar`: el `.sig` generado pasa `VerificarManifiesto` con la clave publica.
- [ ] **Step 2:** Ejecutarlos: fallan.
- [ ] **Step 3: Implementar `cmd/firmar` como funciones testeables y un `main` delgado.**
- [ ] **Step 4:** Ejecutarlos: pasan.
- [ ] **Step 5: Generar las claves reales.**
  - Ejecutar `go run ./cmd/firmar generar ~/.collatech-firma/principal` y `... ~/.collatech-firma/respaldo`.
  - Copiar las dos publicas a `claves.go`.
  - Ninguna privada entra al repo: `.gitignore` anade `privada.txt`.
- [ ] **Step 6: `scripts/empaquetar.sh`.**
  - Compilar los cuatro `actualizacion-*` con el mismo `LDFLAGS`. Los de Windows llevan ademas `-H windowsgui`.
  - Ejecutar `go run ./cmd/firmar manifiesto -dir "$SALIDA" -version "$VERSION"`.
- [ ] **Step 7: `release.yml`, job `paquetes`.**
  - Despues de empaquetar, paso "Firmar el manifiesto": `go run ./cmd/firmar firmar dist/manifiesto.json` con `env: FIRMA_ACTUALIZACIONES: ${{ secrets.FIRMA_ACTUALIZACIONES }}`.
  - En "Reunir los archivos", anadir `-name 'actualizacion-*' -o -name 'manifiesto.json*'`.
- [ ] **Step 8: `docs/ACTUALIZAR.md`.**
  - Como pegar `principal/privada.txt` en GitHub: Settings, Secrets and variables, Actions, New repository secret, nombre `FIRMA_ACTUALIZACIONES`.
  - Que la de respaldo se guarda fuera de linea y no va a GitHub.
  - Como se usa el boton.
- [ ] **Step 9:** `./scripts/empaquetar.sh 0.0.1-prueba` deja `dist/manifiesto.json` con cuatro entradas. `go test ./cmd/firmar/` pasa.
- [ ] **Step 10:** Commit "Firmar las publicaciones para actualizar desde el panel".

### Task 3: Fuente GitHub

**Files:**
- Create: `internal/actualizar/github.go`
- Test: `internal/actualizar/github_test.go`

**Interfaces:**
- Produce:
  - `type Publicacion struct { Version, URLNotas string; Recursos map[string]string }`: `Recursos` va de nombre a URL de descarga.
  - `type Fuente struct { URLUltima string; Cliente *http.Client; UserAgent string }`
  - `func FuenteOficial(version string) Fuente`: usa la URL de la API de la Global Constraints, un timeout de 15 s y el User-Agent `CollaTech-Agent/<version>`.
  - `func (f Fuente) Ultima(ctx context.Context) (Publicacion, error)`: lee `tag_name`, `html_url` y `assets[].name/browser_download_url`.
  - `func (f Fuente) Descargar(ctx context.Context, url, destino string, max int64) error`: escribe en `destino+".parcial"` y renombra al terminar. Si pasa de `max`, da error.

- [ ] **Step 1: Tests con `httptest.Server`.**
  - `TestUltima`: una respuesta con `tag_name` `v1.7.0` da `Version=="1.7.0"` y los recursos mapeados.
  - `TestUltimaErrorHTTP`: un 403 da un error que contiene `"403"`.
  - `TestDescargarLimite`: con 2 KB y `max` 1 KB da error, y no queda el destino.
- [ ] **Step 2:** Ejecutarlos: fallan.
- [ ] **Step 3:** Implementar.
- [ ] **Step 4:** Ejecutarlos: pasan.
- [ ] **Step 5:** Commit "Actualizar: consultar y descargar de GitHub".

### Task 4: Sistema, Orden y Actualizador

**Files:**
- Create: `internal/actualizar/sistema.go`, `sistema_windows.go`, `sistema_linux.go`, `sistema_otros.go`, `actualizador.go`
- Test: `internal/actualizar/actualizador_test.go`

**Interfaces:**
- Consume: Tareas 1 y 3.
- Produce:
  - `func Plataforma() string`: `runtime.GOOS + "-" + runtime.GOARCH`.
  - La interfaz `Sistema`:

    ```go
    type Sistema interface {
        Soportado() (bool, string)                  // false + motivo (macOS, no corre como servicio)
        RutaEjecutable() (string, error)            // el exe que usa el servicio
        Parar(ctx context.Context) error
        Arrancar(ctx context.Context) error
        Lanzar(ordenRuta string) error              // pide aplicar la orden fuera del servicio
    }
    ```

  - `func SistemaActual(dataDir string) Sistema`, segun la plataforma:
    - **Windows:** `mgr.Connect`, servicio `CollaTechAgent`.
      - `RutaEjecutable` saca la ruta de `BinaryPathName`, entre comillas o hasta `.exe`.
      - `Soportado` devuelve false si el servicio no existe.
      - `Lanzar` copia el exe actual a `<datos>/actualizacion/aplicador.exe` y lo ejecuta con `--aplicar-actualizacion <orden>`. Usa `DETACHED_PROCESS|CREATE_NEW_PROCESS_GROUP|CREATE_BREAKAWAY_FROM_JOB` y, si falla `CREATE_BREAKAWAY_FROM_JOB`, reintenta sin esa bandera.
    - **Linux:**
      - `RutaEjecutable` es `os.Executable()` resuelto.
      - `Parar` y `Arrancar` son `systemctl stop|start collatech-agent`.
      - `Lanzar` renombra `orden.json.tmp` a `orden.json` (lo recoge la unidad `.path`).
      - `Soportado` mira que exista `collatech-agent-actualizar.path` en `/lib/systemd/system`, `/usr/lib/systemd/system` o `/etc/systemd/system`. Si no esta, el motivo es: "instala una vez el .deb o el paquete nuevo a mano".
    - **Otros:** `Soportado()` devuelve `false, "en este sistema se actualiza con el paquete a mano"`.
  - `type Orden struct { Version, Plataforma, Nuevo, Manifiesto, Firma, Config, Datos, Servicio string }`. Las rutas son absolutas y se guardan en JSON en `<datos>/actualizacion/orden.json`.
  - `type Estado struct`, con JSON:
    - `Fase string` `json:"fase"`: `""`, `"descargando"`, `"verificando"`, `"esperando_cola"`, `"instalando"` o `"error"`.
    - `Version string` `json:"version,omitempty"`
    - `Error string` `json:"error,omitempty"`
  - `type Resultado struct`, con JSON:
    - `Desde string` `json:"desde"`
    - `Hasta string` `json:"hasta"`
    - `OK bool` `json:"ok"`
    - `Error string` `json:"error,omitempty"`
    - `Fecha time.Time` `json:"fecha"`
  - `type Actualizador struct`, creado con `func NuevoActualizador(o Opciones) *Actualizador`.
    - `Opciones{ Version, DataDir, ConfigPath string; Fuente Fuente; Sistema Sistema; Claves []ed25519.PublicKey; ColaVacia func() bool }`
  - `func (a *Actualizador) Buscar(ctx) (Disponible, error)`
    - `Disponible{ VersionActual string; Nueva *Nueva }`
    - `Nueva{ Version, URLNotas, Manual string; ManualMotivo string }`
    - `Buscar` descarga y verifica el manifiesto para rellenar `Manual` y `Minima`. Si `Comparar(actual, minima) < 0`, pone `ManualMotivo`.
  - `func (a *Actualizador) Instalar(version string) error`
    - Valida, marca la fase y sigue en una goroutine.
    - Si ya hay una en curso: `"ya hay una actualizacion en curso"`.
  - `func (a *Actualizador) Estado() Estado`
  - `func (a *Actualizador) UltimoResultado() *Resultado`: lee `<datos>/actualizacion/resultado.json`.
  - La goroutine de `Instalar`:
    1. descarga el manifiesto, la firma y el `actualizacion-<plataforma>` a `<datos>/actualizacion/`;
    2. `VerificarManifiesto` y `VerificarArchivo`;
    3. espera hasta 30 s a `ColaVacia()`;
    4. escribe `orden.json.tmp`;
    5. llama a `Sistema.Lanzar`.

    Si falla algun paso, la fase pasa a `"error"` con el motivo.

- [ ] **Step 1: Tests con Fuente sobre `httptest` y un `Sistema` de mentira.**
  - `TestBuscarNoHayNueva`
  - `TestBuscarHayNueva`
  - `TestBuscarManualPorMinima`
  - `TestInstalarArchivoCorrupto`: SHA256 distinto da fase `error`, `Lanzar` no se llama y el error contiene `"no coincide"`.
  - `TestInstalarSoloUnaVez`
  - `TestInstalarLanzaOrden`: la orden escrita tiene rutas absolutas y `Version` correcta.
- [ ] **Step 2:** Ejecutarlos: fallan.
- [ ] **Step 3:** Implementar. `GOOS=windows go vet ./internal/actualizar/` tiene que pasar.
- [ ] **Step 4:** `go test ./internal/actualizar/ -race` pasa.
- [ ] **Step 5:** Commit "Actualizar: buscar e instalar".

### Task 5: Aplicar con vuelta atras

**Files:**
- Create: `internal/actualizar/aplicar.go`
- Modify: `cmd/server/main.go`
- Test: `internal/actualizar/aplicar_test.go`

**Interfaces:**
- Consume:
  - `Orden`, `Sistema`, `VerificarManifiesto`, `VerificarArchivo`, `Resultado` (Tareas 1 y 4);
  - `config.Load`, para leer el puerto y TLS de `Orden.Config`.
- Produce:
  - `func Aplicar(ctx context.Context, ordenRuta string, sis Sistema, claves []ed25519.PublicKey, salud func(ctx context.Context) (string, error)) Resultado`
  - `func SaludLocal(configPath string) func(ctx context.Context) (string, error)`: hace GET a `http(s)://127.0.0.1:<puerto>/health` y devuelve `data.version`. Con TLS, salta la verificacion del certificado: es localhost y su propio certificado.
  - En `cmd/server`, el flag `--aplicar-actualizacion <orden.json>` ejecuta `Aplicar(...)` con `SistemaActual` y `ClavesOficiales()`, escribe el resultado y sale.

- `Aplicar`, en orden:
  1. renombra `orden.json` a `orden.en-curso.json`, para que la unidad `.path` no se dispare otra vez;
  2. vuelve a verificar la firma y el SHA256 de `Nuevo`. La plataforma tiene que ser la propia y la version, mayor que la del ejecutable actual;
  3. copia `config.json`, `storage/settings.json` y `storage/jobs.json` (los que existan) a `<datos>/actualizacion/copia/`;
  4. llama a `Parar`;
  5. renombra el exe a `<exe>.anterior`, copia `Nuevo` al sitio del exe (permisos 0755) y llama a `Arrancar`;
  6. reintenta `salud` cada 2 s hasta 60 s. Vale si responde la `Version` de la orden;
  7. **si no vale**:
     - `Parar`;
     - restaura el exe desde `.anterior` y la copia de los datos;
     - `Arrancar`;
     - `Resultado{OK:false, Error: "la version X no respondio; se volvio a la Y"}`;
  8. escribe `resultado.json` y borra `orden.en-curso.json`. En el exito tambien borra `<exe>.anterior` y la descarga.

- [ ] **Step 1: Tests con carpeta temporal, `Sistema` de mentira y `salud` de mentira.**
  - `TestAplicarExito`: el exe queda con el contenido nuevo y `resultado.json` dice `ok:true`.
  - `TestAplicarVuelveAtrasSiNoResponde`: `salud` siempre falla, con un plazo corto inyectado.
    - El exe vuelve al contenido viejo.
    - Un `config.json` que la version nueva tocase durante la prueba queda restaurado.
    - Hay `ok:false`.
  - `TestAplicarVuelveAtrasSiVersionDistinta`
  - `TestAplicarRechazaOrdenManipulada`: el SHA256 del `Nuevo` no coincide. No se llama a `Parar` y el resultado es un error.
- [ ] **Step 2:** Ejecutarlos: fallan.
- [ ] **Step 3: Implementar.** El plazo de 60 s y el intervalo de 2 s van en campos no exportados que las pruebas acortan.
- [ ] **Step 4:** Ejecutarlos: pasan con `-race`. `GOOS=windows go build ./cmd/server` compila.
- [ ] **Step 5:** Commit "Actualizar: aplicar con vuelta atras".

### Task 6: API, panel y /health

**Files:**
- Create: `internal/api/handlers_actualizar.go`
- Modify:
  - `internal/api/server.go`: rutas y el campo `Actualizador *actualizar.Actualizador`;
  - `internal/api/middleware.go`;
  - `internal/api/handlers_pages.go`;
  - `internal/api/web/index.html`;
  - `cmd/server/main.go`: crear el `Actualizador`, con `ColaVacia` hecho a partir de `queue.List()` (sin `Pending` ni `Printing`).
- Test: `internal/api/actualizar_test.go`

**Interfaces:**
- Consume: `Actualizador.Buscar/Instalar/Estado/UltimoResultado`, `Sistema.Soportado` (Tarea 4).
- Produce:
  - `GET /api/actualizacion` responde `{version_actual, plataforma, soportado, motivo, estado, ultimo_resultado}`.
  - `POST /api/actualizacion/buscar` responde `{version_actual, nueva:{version, url_notas, manual, manual_motivo}|null}`.
  - `POST /api/actualizacion/instalar` recibe el cuerpo `{"version":"X"}` y responde 202 `{ok:true}`.
  - Las tres van en `adminPaths`, y las dos POST tambien en `adminWritePaths`.
  - `GET /health` responde `{"ok":true,"message":"healthy","data":{"version":"<Version>"}}`.

- [ ] **Step 1: Tests.**
  - `TestHealthIncluyeVersion`.
  - `TestActualizacionSoloLocal`: con `RemoteAddr` de LAN y token valido da 403.
  - `TestActualizacionSinActualizador`: con `Actualizador` nil, `soportado:false` y sin panic.
  - Ampliar `TestElPanelConectaSusBotonesAlCargar` con `$("buscarActualizacion").addEventListener(`.
- [ ] **Step 2:** Ejecutarlos: fallan.
- [ ] **Step 3: Implementar los handlers.**
- [ ] **Step 4: Panel, tarjeta "Actualizaciones" en la pestana Estado.**
  - Muestra "Version instalada: X" y el boton `id="buscarActualizacion"`.
  - Resultados posibles:
    - "Tienes la última versión";
    - "Hay una versión nueva: X (ver novedades)" con el boton `id="instalarActualizacion"`;
    - texto manual si lo hay.
  - Al pulsar Instalar, `confirm()` con el aviso: "No se imprimirá durante unos segundos".
  - Durante la instalacion consulta `GET /api/actualizacion` cada 1,5 s y muestra la fase:
    - Descargando
    - Verificando firma
    - Esperando a que termine la cola
    - Instalando y reiniciando
  - Cuando la conexion se cae, reintenta `/health` hasta 90 s y muestra `ultimo_resultado`:
    - "Actualizado a X";
    - o "No se pudo: … Se volvió a la Y".
  - Si `soportado` es false, muestra el `motivo` y no muestra los botones.
  - Abierto desde otra PC (`GET` da 403): "Las actualizaciones se hacen desde la PC del agente".
- [ ] **Step 5:** `go test ./... -race` pasa.
- [ ] **Step 6: Prueba en el navegador con `playwright-cli`.**
  - Montaje: agente local en el puerto 18799 y `Fuente` apuntando a un `httptest`/servidor estatico local con una Release firmada con clave de prueba. Para eso hay un flag oculto de pruebas, en la Tarea 7.
  - Hay que ver: "Hay una versión nueva", el progreso y el error de firma con un binario cambiado.
- [ ] **Step 7:** Commit "Panel: buscar e instalar actualizaciones".

### Task 7: Linux de punta a punta: unidades, `--install` y CI

**Files:**
- Modify:
  - `scripts/empaquetar.sh`: dos unidades nuevas en el `.deb`, y el postinst las habilita;
  - `cmd/server/service_unix.go`: `--install` tambien escribe y habilita las dos unidades;
  - `cmd/server/main.go`: modo de pruebas;
  - `.github/workflows/ci.yml`.
- Create: `internal/actualizar/pruebas.go`, con la etiqueta de compilacion `actualizarprueba`.
- Test: `cmd/server/service_unix_test.go` (texto de las unidades) y el job de CI `actualizar-e2e`.

**Interfaces:**
- Produce:
  - Unidad `collatech-agent-actualizar.path`:

    ```
    [Unit]
    Description=CollaTech Agent - vigila ordenes de actualizacion
    [Path]
    PathExists=<datos>/actualizacion/orden.json
    [Install]
    WantedBy=multi-user.target
    ```

  - Unidad `collatech-agent-actualizar.service`:
    - `Type=oneshot`;
    - `ExecStart=<bin> --aplicar-actualizacion <datos>/actualizacion/orden.json`;
    - sin `ProtectSystem`, como root.
  - Rutas de `<datos>`: `/var/lib/collatech` en el `.deb`; `paths.DataDir` en `--install`.
  - Con la etiqueta `actualizarprueba`:
    - variable `COLLATECH_URL_ACTUALIZACIONES`, que cambia `URLUltima`;
    - variable `COLLATECH_CLAVE_PRUEBA`, una publica en base64 que se anade a las oficiales.

    Sin la etiqueta, ninguna de las dos existe: los binarios publicados no las leen.
- Job `actualizar-e2e` en `ci.yml`:
  1. con una clave de prueba generada en el job, construir dos `.deb`:
     - `1.0.0` con la etiqueta, la URL de prueba y la clave de prueba en un drop-in de systemd (`Environment=`);
     - `1.0.1`, solo sus `actualizacion-linux-amd64` y manifiesto, firmados con la clave de prueba;
  2. servir una Release falsa con `python3 -m http.server`: un `latest.json` con forma de la API de GitHub y los archivos;
  3. instalar el `.deb` `1.0.0` y comprobar que `/health` dice `1.0.0`;
  4. `curl -X POST /api/actualizacion/instalar` y, en un bucle de hasta 90 s, comprobar que `/health` dice `1.0.1`;
  5. vuelta atras: publicar `1.0.2` con un binario que sale con error y pedir la instalacion. `/health` vuelve a `1.0.1` y `resultado.json` dice `ok:false`;
  6. `systemctl is-active collatech-agent` responde OK al final.

- [ ] **Step 1:** Test `TestUnidadesActualizar` con el texto de las dos unidades, para rutas dadas.
- [ ] **Step 2:** Ejecutarlo: falla.
- [ ] **Step 3: Implementar.**
  - Unidades en `service_unix.go` y en `empaquetar.sh`.
  - En el `postinst`: `systemctl enable --now collatech-agent-actualizar.path`.
  - En el `prerm`: deshabilitarla.
  - Ademas, `pruebas.go`.
- [ ] **Step 4:** `go test ./cmd/server/` pasa. Con `./scripts/empaquetar.sh 0.0.1-prueba`, `dpkg-deb -c` lista las dos unidades.
- [ ] **Step 5:** Escribir el job `actualizar-e2e`.
- [ ] **Step 6:** Commit, push a `main` y comprobar en GitHub Actions que `actualizar-e2e` pasa. Si falla, se corrige hasta que pase.
- [ ] **Step 7:** Commit de las correcciones, si las hubo.

### Task 8: Publicar la version con el boton

**Files:**
- Modify:
  - `Makefile`, `collatech-sdk/package.json`, `collatech-sdk/package-lock.json`;
  - `ANGULAR.md`, `ENTREGA_CLIENTE/docs/ANGULAR.md`;
  - el `.tgz` del SDK en `ENTREGA_CLIENTE/angular/` e `internal/api/web/sdk/`.

  Es el mismo patron que el commit "Version 1.6.3".

- [ ] **Step 1: El usuario configura el secreto.**
  - Darle el contenido de `~/.collatech-firma/principal/privada.txt` y los pasos de `docs/ACTUALIZAR.md`.
  - Esperar a que confirme que el secreto `FIRMA_ACTUALIZACIONES` esta creado.
- [ ] **Step 2: Comprobaciones.**
  - Subir la version a 1.6.4.
  - `make version-check fmt vet`, `go test ./... -race` y `./scripts/comprobar-ejemplos.sh`.
  - `./scripts/empaquetar.sh 1.6.4`: la comprobacion del instalador embebido pasa y hay manifiesto.
- [ ] **Step 3:** Commit "Version 1.6.4", etiqueta `v1.6.4` y push.
- [ ] **Step 4: Comprobar la Release publicada.**
  - Esperar a que termine.
  - Descargar `manifiesto.json` y `.sig` y verificarlos con la clave publica oficial, usando `go run ./cmd/firmar` o una prueba.
  - Comprobar que el SHA256 de `actualizacion-linux-amd64` coincide.
- [ ] **Step 5: Dar al usuario los pasos para el primer salto a mano en Windows y Ubuntu.**
  - En Ubuntu: `! sudo dpkg -i collatech-agent_1.6.4_amd64.deb`.
  - Explicar que el boton se podra usar para la siguiente version.
