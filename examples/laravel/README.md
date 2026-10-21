# Imprimir desde Laravel en la nube

Tu Laravel está en un hosting o VPS y las impresoras en el local, detrás de
su router. Laravel no puede llamar al agente, así que **llama el agente**:

```text
  Local (192.168.1.x)                        Internet
 ┌──────────────────────┐                ┌──────────────────────┐
 │ CollaTech Agent      │  cada 3 s      │ Laravel              │
 │  ├─ caja   (red)     │ ─ GET /jobs ─▶ │  collatech_trabajos  │
 │  └─ cocina (USB)     │ ◀── trabajos ─ │                      │
 │                      │ ─ /status ───▶ │  printed / failed    │
 │                      │  cada 30 s     │                      │
 │                      │ ─ /heartbeat ▶ │  collatech_agentes   │
 └──────────────────────┘                └──────────────────────┘
```

- **No hay que abrir puertos** en el router del local, ni tener IP fija.
- **Laravel sabe en todo momento** si el agente de cada local está encendido
  y cómo está cada impresora (en línea, papel, tapa), sin imprimir nada.
- **Nada se imprime dos veces**, aunque se corte internet o el agente se
  reinicie a mitad.
- Si el local está apagado, los trabajos esperan y salen al encender. Si no
  quieres que una comanda salga tres horas tarde, ponle caducidad.

Probado con Laravel 13 y PHP 8.3; usa lo normal de Laravel 11 en adelante.

## Instalar (5 minutos)

**1. Copia los archivos** de esta carpeta en tu proyecto, respetando las
rutas:

```text
app/Console/Commands/CollatechAgenteCommand.php
app/Http/Controllers/CollatechAgenteController.php
app/Http/Middleware/AutenticarAgenteCollatech.php
app/Models/CollatechAgente.php
app/Models/CollatechTrabajo.php
app/Providers/CollatechServiceProvider.php
app/Services/Collatech.php
app/Services/ImpresoraNoDisponible.php
database/migrations/2026_10_21_000000_create_collatech_tables.php
routes/collatech.php
```

**2. Registra el provider** en `bootstrap/providers.php`:

```php
return [
    App\Providers\AppServiceProvider::class,
    App\Providers\CollatechServiceProvider::class,   // ← esta línea
];
```

**3. Crea las tablas:**

```bash
php artisan migrate
```

**4. Da de alta cada local:**

```bash
php artisan collatech:agente "Local Centro"
```

Te muestra la URL y el token de ese local. **Copia el token en ese momento:**
solo se guarda su hash y no se puede volver a ver. Si lo pierdes, da de alta
el local otra vez.

**5. En la PC del local**, abre `http://localhost:18743/panel`, pestaña
**Estado**, sección *Conexión con tu sistema en internet*: pega la URL
(`https://tu-app.com/api/collatech`) y el token, y pulsa **Guardar y
conectar**. En unos segundos debe decir `CONECTADO`.

Para comprobarlo desde Laravel:

```bash
php artisan collatech:agente
```

```text
+----+--------------+-----------+-----------------+--------------------------+
| id | local        | agente    | ultimo contacto | estaciones               |
+----+--------------+-----------+-----------------+--------------------------+
| 1  | Local Centro | ENCENDIDO | 5 seconds ago   | caja: lista, cocina: lista |
+----+--------------+-----------+-----------------+--------------------------+
```

## Imprimir

```php
use App\Models\CollatechAgente;
use App\Services\Collatech;

$local = CollatechAgente::where('nombre', 'Local Centro')->first();

// Un recibo en la caja y la comanda en cocina: salen a la vez.
Collatech::imprimir($local, 'caja', [
    'title' => 'RECIBO',
    'lines' => [
        ['type' => 'table', 'table' => [
            'header' => true, 'border' => true,
            'columns' => [['text' => 'Producto', 'width' => 30], ['text' => 'Bs', 'width' => 10, 'align' => 'right']],
            'rows' => [['Producto', 'Bs'], ['Café con leche', '24.00']],
        ]],
        ['text' => 'TOTAL Bs 24.00', 'align' => 'right', 'bold' => true],
        ['type' => 'qr', 'qr' => ['data' => 'https://tu-app.com/f/123', 'size' => 4]],
    ],
]);

Collatech::imprimir($local, 'cocina', [
    'lines' => [
        ['text' => 'COMANDA #42 - MESA 5', 'bold' => true, 'size' => 'double'],
        ['text' => '2 x Café con leche', 'size' => 'tall'],
    ],
], caducaEn: now()->addMinutes(10));
```

`'caja'` y `'cocina'` son los nombres de las **estaciones** del agente de ese
local (pestaña *Estaciones* del panel). Los datos son los mismos que acepta el
agente en `POST /api/print/ticket`; el campo `printer` lo pone `Collatech`.
Todo lo que admite un ticket está en `JSON-REFERENCIA.md`.

Otros tipos, con el cuarto parámetro: `text`, `template`, `html`, `layout`,
`image`, `logo` y `raw`.

```php
Collatech::imprimir($local, 'caja', ['text' => "Hola\n"], 'text');
```

## Saber si se puede imprimir

```php
$local->enLinea();                  // ¿la PC del local está encendida?
$local->puedeImprimir('cocina');    // ¿y esa impresora, con papel y tapa cerrada?
$local->problema('cocina');         // null, o el motivo en palabras:
// "La PC del local 'Local Centro' no responde (ultimo contacto: 3 minutes ago)."
// "La impresora 'cocina': sin papel."
$local->estacion('cocina');         // todo el detalle que mandó el agente
```

El agente late cada 30 segundos. Se da por apagado a los **90 segundos** sin
latido (`CollatechAgente::SEGUNDOS_SIN_LATIDO`): así se toleran dos latidos
perdidos por un corte de internet sin dar falsas alarmas.

Si prefieres no encolar nada que no vaya a salir:

```php
use App\Services\ImpresoraNoDisponible;

try {
    Collatech::imprimirSiSePuede($local, 'cocina', $comanda);
} catch (ImpresoraNoDisponible $e) {
    return back()->withErrors($e->getMessage());
}
```

## Saber si salió

`imprimir()` devuelve el trabajo (`CollatechTrabajo`). Su `estado`:

| Estado | Qué significa |
|---|---|
| `pending` | Esperando a que el agente lo recoja |
| `received` | El agente lo tiene y lo está imprimiendo |
| `printed` | Salió |
| `failed` | No salió; el motivo está en `error` (por ejemplo `cocina: sin papel`) |
| `expired` | Caducó antes de que el agente lo recogiera |

Si la respuesta al usuario depende de que haya salido, espera hasta unos
segundos:

```php
$trabajo = Collatech::esperar(Collatech::imprimir($local, 'caja', $recibo), 15);

if (! $trabajo->impreso()) {
    // $trabajo->estado, $trabajo->error
}
```

Para avisar de los fallos sin esperar, escucha el modelo:

```php
// En AppServiceProvider::boot()
CollatechTrabajo::updated(function (CollatechTrabajo $t) {
    if ($t->wasChanged('estado') && $t->estado === CollatechTrabajo::FALLIDO) {
        // notificar al encargado del local
    }
});
```

## Que el agente esté siempre encendido

En la PC del local el agente es un **servicio del sistema**: arranca solo al
encender la PC, antes de que nadie inicie sesión, y vuelve a arrancar solo si
se cae.

- **Linux:** unidad systemd con `Restart=always` y sin límite de reintentos.
  `systemctl status collatech-agent` lo muestra.
- **Windows:** servicio `CollaTechAgent` en inicio automático, con reinicio a
  los 5 segundos ante cualquier fallo. Se ve en `services.msc`.

Lo que no puede hacer es seguir funcionando con la PC apagada o sin luz. Para
eso está el latido: Laravel lo sabe a los 90 segundos.

## Lo que pasa cuando algo falla

| Situación | Qué hace |
|---|---|
| Se corta internet en el local | El agente reintenta, cada vez más espaciado hasta 1 minuto. Al volver, recoge lo pendiente |
| El agente se reinicia a mitad de un trabajo | No lo reimprime (pudo haber salido): lo informa como `failed` con el motivo, para que decidas |
| Laravel no recibe la respuesta | El agente se la repite en la siguiente vuelta; no vuelve a imprimir |
| La cola del agente está llena | El trabajo se queda `pending` y se recoge en la siguiente vuelta |
| Impresora sin papel o apagada | `failed` con el motivo exacto, tras los reintentos del agente |
| Token mal pegado | El panel del agente dice "el backend rechazo el token" |

## Las rutas, por si no usas este ejemplo

El agente habla un protocolo de tres rutas, todas con `Authorization:
Bearer <token>`. Si tu backend no es Laravel, implementa estas:

| Ruta | El agente manda | Tu backend contesta |
|---|---|---|
| `GET {url}/jobs` | — | `{"jobs":[{"id":"...","type":"ticket","data":{...}}]}` con lo pendiente |
| `POST {url}/jobs/{id}/status` | `{"status":"received\|printed\|failed","error":"...","local_jobs":[...]}` | 2xx. 404 si no conoces el trabajo |
| `POST {url}/heartbeat` | `{"agent":{"version","hostname","os","started_at","uptime_seconds"},"stations":[...]}` | 2xx |

Un trabajo debe dejar de salir en `/jobs` en cuanto se informe como
`printed` o `failed`. Mientras siga saliendo, el agente no lo reimprime: solo
repite el último estado.
