# Conectar una impresora termica en Ubuntu

Probado en Ubuntu 64 bits con una **Epson TM-T88V** por USB el 3 de octubre
de 2026. Los pasos valen para cualquier termica ESC/POS; donde algo sea
propio de la Epson se dice.

## Lo primero: casi seguro no necesitas el driver del fabricante

Es la confusion mas comun. En la pagina de soporte de Epson para Linux hay
cuatro descargas, y para imprimir desde CollaTech Agent **no hace falta
ninguna**:

| Descarga de Epson | Para que sirve | ¿La necesitas? |
|---|---|---|
| Thermal Linux Driver (`tmt-cups`) | Hace que la impresora salga como impresora del sistema | Opcional |
| JavaPOS ADK for Linux | SDK para programar en Java | No |
| OPOS ADK | Solo Windows | No |
| Advanced Printer Driver (APD) | Solo Windows | No |

El agente genera los bytes ESC/POS el mismo y los manda a la impresora. No
usa el driver para nada: le basta una **cola raw**, que es una cola que pasa
los bytes tal cual sin interpretarlos.

Asi que hay dos caminos, y el corto suele bastar.

## Camino corto: cola raw, sin instalar nada

### 1. Comprobar que Ubuntu ve la impresora

```bash
lsusb | grep -i epson
```

Debe salir algo como:

```
Bus 001 Device 005: ID 04b8:0202 Seiko Epson Corp. UB-U05 (TM-T88IV)
```

Ese `04b8:0202` es el identificador USB. Que ponga **TM-T88IV** cuando tu
impresora es una **T88V** es normal: es el nombre de la interfaz USB, no del
modelo.

### 2. Ver que URI detecta CUPS

```bash
lpinfo -v
```

Busca la linea que empieza por `direct usb://`:

```
direct usb://EPSON/TM-T88V?serial=574A50468415070000
```

**Copia esa URI entera**, con el numero de serie incluido.

### 3. Crear la cola

```bash
sudo lpadmin -p TM-T88V -E -v "usb://EPSON/TM-T88V?serial=574A50468415070000" -m raw
```

- `-p TM-T88V` es el nombre que le das; puede ser el que quieras.
- `-E` la deja activada.
- `-v` es la URI del paso anterior, entre comillas.
- `-m raw` dice que no interprete nada.

CUPS avisa de que *"Raw queues are deprecated"*. Es solo un aviso: la cola se
crea y funciona. Sin `-m raw` tambien funciono.

### 4. Comprobar

```bash
lpstat -p
```

Debe decir que esta *inactiva y activada*, que significa lista.

### 5. Probar antes de meterla en el agente

```bash
# Texto suelto
echo "Prueba" | lp -d TM-T88V

# ESC/POS: inicializa, imprime, avanza y corta
printf '\x1b@Prueba ESC/POS\n\n\n\n\x1d\x56\x00' | lp -d TM-T88V -o raw
```

Si sale papel, ya esta: la impresora funciona y el agente la va a encontrar.

### 6. Darla de alta en el agente

En el panel, pestana **Impresoras**, o desde la pagina de
[Impresoras](http://localhost:18743/impresoras). El destino es el **nombre de
la cola**:

```
TM-T88V
```

## Camino aun mas corto: sin CUPS

Si no quieres cola ninguna, el agente puede escribir directo al dispositivo:

```bash
ls -l /dev/usb/lp*
```

Si aparece `/dev/usb/lp0`, el destino en el agente es:

```
device:///dev/usb/lp0
```

Hace falta que el usuario que corre el agente este en el grupo `lp`:

```bash
sudo usermod -aG lp $USER
```

Y volver a iniciar sesion para que surta efecto. El servicio que instala el
`.deb` ya viene con ese grupo puesto.

## Camino largo: instalar el driver CUPS de Epson

Solo si quieres que la impresora salga en el dialogo de Impresoras de GNOME
como una impresora normal, o si vas a imprimir desde programas que no son
este agente.

El instalador es de 2010 y solo lista Ubuntu 9.04, pero funciona en un Ubuntu
moderno de 64 bits.

```bash
cd ~/Descargas/tm_ba_series_thermal_printer_driver_1100/TM_BA_Series_Thermal_Printer_Driver_1100/tmt-cups-1.1.0.0/tmt-cups
chmod +x install.sh
sudo ./install.sh
```

Elige la **opcion 2** (Ubuntu 9.04 x86_64 DEB), que es la unica que aplica a
64 bits. Espera a `*** The installation finished. ***`.

Instala tres paquetes (`ep-escpos`, `epson-cups-escpos`, `tmt-cups`) y
reinicia los servicios `epurasd` y `cups`.

**Si la ruta tiene espacios**, `cd` falla con *"too many arguments"*. Pon la
ruta entre comillas, o usa la carpeta con guiones bajos.

## Cuando algo no sale

| Sintoma | Por que | Que hacer |
|---|---|---|
| *"No se han encontrado impresoras"* en Configuracion | El dialogo de GNOME no detecta estas USB | Crear la cola con `lpadmin`, como arriba |
| No aparece en la pantalla de Bluetooth | La impresora va por USB | Buscar en Configuracion → Impresoras, o por terminal |
| `cd: too many arguments` | La ruta tiene espacios | Comillas, o la carpeta con guiones bajos |
| `Raw queues are deprecated` | CUPS nuevo lo desaconseja | Es un aviso; la cola funciona |
| `lp: no se ha podido acceder a "archivo.bin"` | El archivo no existe | Usar uno real, o mandar los bytes con `printf` |
| `Bad driver information file ... Utax`, `cups-brf must be called as root` en `/var/log/cups/error_log` | Ruido de otros drivers de CUPS | Ignorar; no afecta |

Si deja de imprimir, revisa en este orden:

1. Papel, tapa cerrada y luz de error apagada.
2. `lpstat -t` y `sudo tail -n 20 /var/log/cups/error_log`.
3. Que el usuario este en el grupo `lp`: `sudo usermod -aG lp $USER`, y volver a entrar.
4. `ls -l /dev/usb/lp*` para probar a escribir directo al dispositivo.

## Comandos ESC/POS para probar a mano

Utiles para saber si el problema es la impresora o el agente.

| Funcion | Bytes |
|---|---|
| Inicializar | `\x1b\x40` |
| Negrita on / off | `\x1b\x45\x01` / `\x1b\x45\x00` |
| Centrar / izquierda | `\x1b\x61\x01` / `\x1b\x61\x00` |
| Doble tamano / normal | `\x1d\x21\x11` / `\x1d\x21\x00` |
| Cortar papel | `\x1d\x56\x00` |
| Abrir cajon | `\x1b\x70\x00\x19\xfa` |

Un ticket completo a mano:

```bash
printf '\x1b@\x1b\x61\x01\x1b\x45\x01MI NEGOCIO\x1b\x45\x00\n\x1b\x61\x00Producto 1      10.00\nTotal           10.00\n\n\n\n\x1d\x56\x00' | lp -d TM-T88V -o raw
```

Para mandar un archivo de bytes ya generados:

```bash
lp -d TM-T88V -o raw archivo.bin
```

## Y en Windows

Por USB hace falta una cola de impresion, pero el driver **Generic / Text
Only** que ya trae Windows sirve para casi todas:

1. *Configuracion → Bluetooth y dispositivos → Impresoras y escaneres*
2. *Agregar dispositivo* → *Agregar manualmente*
3. *Agregar una impresora local* → puerto **USB001**
4. Fabricante **Generic**, impresora **Generic / Text Only**
5. Ponle un nombre; ese nombre es el destino en el agente

El driver del fabricante solo hace falta en los modelos que lo dicen en la
pagina de [Impresoras](http://localhost:18743/impresoras). La Star TSP100 es
el caso claro: exige su `futurePRNT` y ademas hay que activarle la emulacion
ESC/POS antes de que nada funcione.

**Por red no hace falta driver en ningun sistema.** Si tu impresora tiene
Ethernet o WiFi, esa es la via mas simple: el destino es
`tcp://192.168.1.50:9100` y no se instala nada.
