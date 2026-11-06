# Actualizar el agente desde el panel

Fecha: 2026-11-05 · Estado: borrador para aprobar

## Por que

Hoy, para actualizar el agente hay que ir a cada PC, bajar el zip o el
`.deb` de GitHub y ejecutar el instalador a mano. Con varias cajas y
locales eso no se hace, y los agentes se quedan en versiones viejas con
fallos ya corregidos (como el panel de "Webs autorizadas" roto hasta la
1.6.3).

Lo que se quiere: que quien este delante de la PC del agente abra el panel,
pulse **Buscar actualizacion**, vea si hay una version nueva y, si la hay,
la instale con un boton. En Windows y en Linux, igual.

## Objetivos

- Boton **Buscar actualizacion** en el panel. Consulta GitHub solo cuando se
  pulsa: el agente nunca consulta nada por su cuenta.
- Si hay una version mas nueva, la muestra (numero y enlace a sus notas) con
  un boton **Instalar**.
- Al instalar: descarga el programa de su sistema, **verifica la firma**,
  reemplaza el ejecutable, reinicia el servicio y comprueba que la version
  nueva responde. Si no responde, **vuelve sola a la anterior**.
- Conserva todo: `config.json` (token, webs autorizadas, nube), impresoras
  dadas de alta, registros, certificados. Solo cambia el ejecutable.
- El panel muestra el progreso y el resultado final, tambien despues del
  reinicio ("Actualizado a 1.6.5" o "No se pudo: … Se volvio a la 1.6.4").
- Windows (servicio de Windows) y Linux (systemd: el `.deb` y el
  `INSTALAR.sh`/`--install` del tar.gz), amd64, arm64 y Windows 32 bits.

## No objetivos

- Actualizacion automatica o programada.
- Ordenar actualizaciones desde el backend en la nube a muchos locales.
  (Se puede construir despues encima de esto.)
- macOS: el boton dice que en Mac se actualiza con el paquete a mano.
- Bajar de version desde el panel. Solo se ofrecen versiones mayores.
- Actualizar el SDK de npm o el front de Angular.

## Anti-objetivos (seria un fracaso aunque "funcione")

- Que un archivo que no salio de tus publicaciones llegue a ejecutarse en
  una caja. El agente corre como administrador/root: la firma no es
  opcional, y si no se puede verificar no se instala nada.
- Que una actualizacion deje un local sin imprimir.
- Que se pierda configuracion (el token sobre todo: las demas PCs dejarian
  de poder imprimir).
- Que cualquiera desde la red pueda lanzar una actualizacion.
- Debilitar la proteccion permanente del servicio en Linux
  (`ProtectSystem=strict`) para poder actualizar.

## Restricciones

- El ejecutable del servicio no se puede sobrescribir mientras corre en
  Windows (si se puede renombrar). El cambio lo tiene que hacer un proceso
  distinto del servicio.
- En Linux el servicio esta encerrado: solo escribe en su carpeta de datos y
  en la de configuracion. No puede escribir en `/usr/local/bin` ni tocar
  systemd. Cualquier proceso que lance muere con el servicio al reiniciarlo.
- Instalaciones en Linux con rutas distintas:
  - `.deb`: binario `/usr/local/bin/collatech-agent`, config
    `/etc/collatech/config.json`, datos `/var/lib/collatech`, unidad en
    `/lib/systemd/system/collatech-agent.service`.
  - `--install`: mismo binario, config `/etc/collatech-agent/config.json`,
    datos `/var/lib/collatech-agent`, unidad en
    `/etc/systemd/system/collatech-agent.service`.
- Windows: todo en `C:\Program Files\CollaTech Agent\`. El servicio
  `CollaTechAgent` corre como LocalSystem. Segun como se instalo, su
  `binPath` es solo el `.exe` (INSTALADOR) o el `.exe` con `--config` y
  `--data-dir` (`--install`).
- Las publicaciones salen de GitHub Actions al subir una etiqueta `v*`.
  Hoy suben zip, tar.gz, `.deb`, el SDK y `SHA256SUMS.txt`, pero **no** los
  ejecutables sueltos.
- GitHub limita a 60 consultas por hora sin autenticar por IP. Con un boton
  manual sobra.
- Las PCs que hoy tienen la 1.6.3 o anteriores no tienen el boton: necesitan
  una ultima actualizacion a mano.

## Enfoques considerados

1. **El agente se actualiza a si mismo, con firma y vuelta atras.** Descarga
   solo el ejecutable de su sistema, verifica la firma, y un proceso aparte
   hace el cambio, reinicia y comprueba; si falla, restaura el anterior.
   Igual en los dos sistemas. Coste: el mas trabajo, y una configuracion
   unica de la clave de firma en GitHub.
2. **El agente descarga el instalador oficial y lo ejecuta en silencio**
   (`INSTALADOR.exe` / `.deb`). Reutiliza lo existente, pero no vuelve atras
   si la version nueva falla, obliga a quitar `ProtectSystem=strict` en
   Linux para siempre y solo verifica que el archivo no llego danado.

**Elegido: 1**, por el usuario. Razon: el agente corre como administrador en
cada caja y una actualizacion fallida no puede dejar un local sin imprimir.

## Como funciona (decidido)

### Firma de las publicaciones

- Clave Ed25519. La **privada** vive solo como secreto de GitHub Actions
  (`FIRMA_ACTUALIZACIONES`) y en una copia de seguridad que guarda el
  usuario. Nunca en el repositorio. La **publica** va escrita en el codigo
  del agente.
- Al publicar, el workflow:
  - sube tambien los ejecutables sueltos de cada sistema;
  - genera un manifiesto con la version y el SHA256 de cada ejecutable;
  - lo firma y sube manifiesto y firma a la Release.
- El agente solo acepta un ejecutable si la firma del manifiesto es valida
  con la clave publica que lleva dentro, el SHA256 del archivo coincide con
  el del manifiesto, y la version del manifiesto es la de la Release.
- Se incluye un script para generar la pareja de claves en local y una guia
  de 5 minutos para meter la privada en GitHub.

### Buscar

- `POST /api/actualizacion/buscar`: consulta la ultima Release de
  `yedrick/AgenteImpresora` y la compara con la version propia (semver).
- Responde: version actual, version disponible (si es mayor), enlace a las
  notas, y si hay ejecutable para este sistema y arquitectura.
- Sin internet o con GitHub caido: error claro ("No se pudo consultar
  GitHub: …"), nada mas.

### Instalar

- `POST /api/actualizacion/instalar` con la version que se acepto.
- El agente:
  1. descarga manifiesto, firma y ejecutable a `<datos>/actualizacion/`;
  2. verifica todo;
  3. deja una **orden de actualizacion** (archivo) en esa carpeta;
  4. pide que la aplique un proceso externo al servicio (abajo).
- **Aplicar** lo hace el ejecutable *instalado* con `--aplicar-actualizacion`.
  Vuelve a verificar la firma: no se fia de lo que haya en la carpeta de
  datos. Luego:
  1. guarda el ejecutable actual como `anterior`;
  2. pone el nuevo en su sitio;
  3. reinicia el servicio;
  4. espera hasta 60 s a que `/health` responda y la version sea la nueva;
  5. si no, restaura `anterior`, reinicia y lo apunta como fallo;
  6. escribe el resultado en `<datos>/actualizacion/resultado.json`.
- **Windows:** el servicio lanza ese proceso desacoplado, y el proceso
  sobrevive al stop del servicio. El servicio se para y arranca con el SCM.
- **Linux:** el paquete instala dos unidades nuevas:
  - `collatech-agent-actualizar.path`, que vigila el archivo de orden;
  - `collatech-agent-actualizar.service`, de un solo disparo, como root, sin
    el encierro del servicio principal, que ejecuta `--aplicar-actualizacion`.

  El servicio principal sigue igual de encerrado: solo escribe un archivo en
  su carpeta de datos. Las instalan el `.deb` y `--install`.
- Solo una actualizacion a la vez. Si ya hay una en curso, se rechaza.

### Seguridad de las rutas

- `/api/actualizacion/*` solo responde en la propia PC del agente, como
  `/api/cors` o `/api/token`.
- Aunque alguien lograra lanzar una actualizacion, solo puede instalar una
  version firmada y mayor que la actual.

### Panel

- En la pestana **Estado**, una tarjeta **Actualizaciones**:
  - "Version instalada: 1.6.4" y el boton **Buscar actualizacion**.
  - Resultado: "Tienes la ultima version", o "Hay una version nueva: 1.6.5
    (ver novedades)" con el boton **Instalar 1.6.5**.
  - Al instalar, pide confirmacion y avisa de que no se imprimira durante
    unos segundos.
  - Progreso: Descargando → Verificando firma → Instalando → Reiniciando el
    agente.
  - Durante el reinicio el panel pierde la conexion. Reintenta hasta que
    vuelve y muestra el resultado de `resultado.json`.
- Abierto desde otra PC, la tarjeta dice que se actualiza desde la PC del
  agente.
- En macOS, la tarjeta explica que se actualiza con el paquete.

### Trabajos en cola

- Antes de reiniciar se espera a que la cola de impresion se vacie, como
  maximo 30 s. Lo que quede pendiente se reintenta tras el arranque, como en
  cualquier reinicio.

## Decisiones tras la revision (tomadas por Claude, el usuario puede cambiarlas)

1. **Volver atras restaura tambien los datos.** Antes de cambiar el
   ejecutable, el aplicador copia `config.json`, `storage/settings.json` y
   `storage/jobs.json` a la carpeta de trabajo. Si la version nueva falla la
   comprobacion, se restauran junto con el ejecutable. Asi la version
   anterior nunca se encuentra con datos que no entiende. El token nunca se
   pierde: la copia es del archivo entero.
2. **El manifiesto firmado puede frenar la actualizacion por panel.** Lleva
   `minima` (version minima desde la que se puede actualizar por panel) y
   `manual` (texto; si existe, esta version se instala a mano y el panel lo
   explica en vez de ofrecer Instalar). Sirve para cuando una version cambie
   unidades de systemd, firewall o certificados. El lector ignora los campos
   que no conoce, para que versiones futuras puedan anadir mas.
3. **Dos claves publicas en el agente: principal y de respaldo.** Vale una
   firma de cualquiera de las dos. La de respaldo no se usa nunca en GitHub:
   el usuario la guarda fuera de linea. Si la principal se pierde o se filtra,
   la siguiente version se firma con la de respaldo y trae una principal
   nueva, sin tener que ir caja por caja.
4. **Windows: se actualiza el ejecutable que use el servicio, donde este.**
   El aplicador lee la ruta real del ejecutable en la configuracion del
   servicio `CollaTechAgent` (no supone `Program Files`). Un agente de
   32 bits sigue siendo de 32 bits: se descarga el del mismo sistema y la
   misma arquitectura con que se compilo el que corre. Si el agente no corre
   como servicio (abierto a mano), el panel dice que se actualiza a mano.
5. **Linux `.deb`: se cambia el ejecutable directamente, sin dpkg.** El
   mecanismo es el mismo para el `.deb` y para `--install`, y la vuelta atras
   es simple. Efecto conocido y aceptado: `dpkg -l` sigue mostrando la
   version del `.deb` instalado, hasta que se instale otro `.deb` a mano. Se
   anota en la guia.
6. **Sin el secreto de firma, la publicacion falla.** Si
   `FIRMA_ACTUALIZACIONES` no esta configurado, el workflow de Release se
   detiene con un mensaje que dice como configurarlo. Nunca sale una version
   que el boton no pueda instalar sin que nadie se entere.
7. `/health` pasa a incluir la version, para que el aplicador y el panel
   sepan cual responde.

## Que queda para quien lo construya

- El formato exacto del manifiesto y de la orden.
- Nombres de los ejecutables sueltos en la Release.
- Como lanzar el proceso desacoplado en Windows.
- Rutas exactas de `anterior` y de la carpeta de trabajo.
- Textos del panel (en el mismo tono que el resto).
- Pruebas:
  - unitarias: comparar versiones, verificar firma, manifiesto, cambio y
    vuelta atras, con un sistema de archivos y un control de servicios de
    mentira;
  - en CI: actualizar de punta a punta en Linux, instalando un `.deb` y
    apuntando a una Release de prueba local firmada con una clave de prueba.

    La URL de las Releases solo se puede cambiar para pruebas, y aun asi la
    firma manda.
- Windows de punta a punta no se puede probar en CI ni en esta maquina: lo
  probara el usuario en su PC. Hay que darle los pasos.
