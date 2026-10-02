# Ejemplos

## Tickets por JSON — `tickets/`

Se mandan tal cual a `POST /api/print/ticket`:

```bash
curl -X POST http://localhost:18743/api/print/ticket \
  -H "Content-Type: application/json" \
  -d @examples/tickets/factura.json
```

| Archivo | Qué muestra |
|---|---|
| `factura.json` | 80 mm, tabla con cabecera, total enmarcado, QR, modo compacto |
| `comanda.json` | 58 mm, letra de doble alto, texto invertido, código de barras, sin cortar |
| `recibo-girado.json` | Impresión girada 180°, texto a doble tamaño, espacio para firma |

Desde otra PC hay que añadir el token:

```bash
curl -X POST http://192.168.1.50:18743/api/print/ticket \
  -H "Authorization: Bearer TU_TOKEN" \
  -H "Content-Type: application/json" \
  -d @examples/tickets/factura.json
```

## Tickets por HTML — `html/`

El contenido del archivo va en el campo `html`:

```bash
python3 - <<'PY' | curl -X POST http://localhost:18743/api/print/html \
  -H "Content-Type: application/json" -d @-
import json
print(json.dumps({
    "printer": "caja", "width": 576, "cut": "partial", "compact": True,
    "html": open("examples/html/factura.html").read(),
}))
PY
```

## Angular

`angular/collatech-print.service.ts` es un servicio listo para copiar.
La guía completa está en [ANGULAR.md](../ANGULAR.md).

## Diseñador visual

Para armar un ticket sin escribir JSON a mano:

```text
http://localhost:18743/designer
```

Se diseña con el ratón y la pestaña **JSON** da el cuerpo exacto que hay que
enviar.
