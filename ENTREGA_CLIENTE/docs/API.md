# CollaTech Agent API

Base URL: `http://localhost:18743`

All endpoints return:

```json
{
  "ok": true,
  "message": "optional",
  "data": {}
}
```

Errors return:

```json
{
  "ok": false,
  "error": "description"
}
```

## Endpoints

- `GET /health`
- `GET /api/status`
- `GET /api/printers`
- `GET /api/printer-aliases`
- `POST /api/printer-aliases`
- `POST /api/print/text`
- `POST /api/print/ticket`
- `POST /api/print/html`
- `POST /api/print/image`
- `POST /api/print/raw`

## Printer Aliases

Los alias permiten manejar estaciones logicas: `cocina`, `recepcion`, `facturas`, `pagos`.

```json
[
  { "name": "cocina", "printer": "EPSON Cocina,EPSON Barra", "description": "Pedidos" },
  { "name": "facturas", "printer": "EPSON Caja", "description": "Facturacion" }
]
```

Al imprimir se puede enviar:

```json
{ "printer": "cocina", "text": "Pedido #1001", "cut": true }
```

El agente resuelve `cocina` a una o varias impresoras reales antes de encolar el trabajo. Si hay varias separadas por coma, crea un trabajo para cada impresora. La cola trabaja en paralelo entre impresoras distintas y mantiene orden por cada impresora.
