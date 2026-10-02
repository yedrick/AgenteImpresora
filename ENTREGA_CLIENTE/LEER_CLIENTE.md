# CollaTech Agent - Guia para cliente

## 1. Instalacion

1. Descomprimir esta carpeta en la PC donde esta conectada la impresora.
2. Doble clic sobre `INSTALADOR.exe`.
3. Windows va a pedir permiso de administrador (aviso de Windows) — aceptar.
4. Confirmar la instalacion en la ventana que aparece y esperar a que termine.

El instalador configura:

- CollaTech Agent
- Puerto `18743`
- Firewall de Windows
- Autoarranque al iniciar Windows
- Panel web local

## 2. Probar en la misma PC

Abrir en el navegador:

```text
http://localhost:18743/diagnostico
```

Tambien se puede abrir:

```text
http://localhost:18743/panel
http://localhost:18743/health
```

Si `/health` responde:

```json
{"ok":true,"message":"healthy"}
```

el agente esta funcionando.

## 3. Probar desde otra PC o celular

En la PC instalada, abrir:

```text
http://localhost:18743/diagnostico
```

Buscar la IP de la PC. Ejemplo:

```text
192.168.1.250
```

Desde otra PC o celular conectado a la misma red WiFi/LAN, abrir:

```text
http://192.168.1.250:18743/health
http://192.168.1.250:18743/panel
```

Cambiar `192.168.1.250` por la IP real que muestre el diagnostico.

## 4. Integracion con sistema web

La URL base del agente sera:

```text
http://IP-DE-LA-PC:18743
```

Ejemplo:

```text
http://192.168.1.250:18743
```

Endpoint de prueba:

```text
GET /health
```

Ejemplo:

```text
http://192.168.1.250:18743/health
```

## 5. Si no conecta desde celular u otra PC

Ejecutar como administrador:

```text
ABRIR_FIREWALL_LAN.bat
```

Luego probar de nuevo:

```text
http://IP-DE-LA-PC:18743/health
```

Si todavia no funciona, ejecutar:

```text
DIAGNOSTICO_RED_COLLATECH.bat
```

Y enviar el resultado a soporte.

## 6. Reglas de firewall que crea

El instalador crea estas reglas:

- `GOServer18743`: permite entrada TCP por el puerto `18743`.
- `CollaTech Agent 18743`: regla adicional de compatibilidad.
- `CollaTech Agent App`: permite el programa `CollaTechAgent.exe`.

La regla principal es:

```text
GOServer18743
```

## 7. Detener el agente

CollaTech Agent corre como Servicio de Windows (arranca solo al encender la PC). Para detenerlo temporalmente, ejecutar como administrador:

```text
DETENER_COLLATECH.bat
```

Se vuelve a levantar solo si reinicias la PC o el servicio. Para pausarlo de forma permanente hasta que lo actives de nuevo, ejecutar como administrador:

```text
sc config CollaTechAgent start= demand
sc stop CollaTechAgent
```

Y para reactivarlo:

```text
sc config CollaTechAgent start= auto
sc start CollaTechAgent
```

## 8. Soporte

Para diagnostico completo abrir:

```text
http://localhost:18743/diagnostico
```

Usar el boton:

```text
Descargar JSON
```

y enviar ese archivo a soporte.
