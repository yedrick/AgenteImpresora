@echo off
echo ==========================================
echo   PRUEBA DE IMPRESION - CollaTech Agent
echo ==========================================
echo.
echo Verificando conexion con el agente...
echo.

curl -s http://localhost:18743/health
echo.
echo.

echo [1] Imprimiendo TICKET de prueba...
curl -s -X POST http://localhost:18743/api/print/ticket ^
  -H "Content-Type: application/json" ^
  -d "{\"printer\":\"Impresora1\",\"title\":\"MI TIENDA C.A.\",\"lines\":[{\"text\":\"Factura #001234\",\"align\":\"center\",\"bold\":true},{\"text\":\"Fecha: 2026-05-29\",\"align\":\"left\"},{\"text\":\"--------------------------------\",\"align\":\"left\"},{\"text\":\"Cafe ............ Bs 5.00\",\"align\":\"left\"},{\"text\":\"Te .............. Bs 3.00\",\"align\":\"left\"},{\"text\":\"Jugo ............ Bs 4.00\",\"align\":\"left\"},{\"text\":\"--------------------------------\",\"align\":\"left\"},{\"text\":\"TOTAL: Bs 12.00\",\"align\":\"right\",\"bold\":true}],\"cut\":true}"
echo.
echo.

echo [2] Imprimiendo RECIBO con template...
curl -s -X POST http://localhost:18743/api/print/template ^
  -H "Content-Type: application/json" ^
  -d "{\"printer\":\"Impresora1\",\"template\":\"recibo\",\"data\":{\"empresa\":\"Mi Tienda C.A.\",\"cliente\":\"Maria Lopez\",\"total\":\"Bs 250.00\",\"mensaje\":\"Gracias por su compra!\"},\"cut\":true}"
echo.
echo.

echo [3] Imprimiendo HTML nativo...
curl -s -X POST http://localhost:18743/api/print/html ^
  -H "Content-Type: application/json" ^
  -d "{\"printer\":\"Impresora1\",\"html\":\"<center><b>COMPROBANTE DE PAGO</b></center><hr><p>Factura: #001234</p><p>Cliente: Juan Perez</p><p>Fecha: 2026-05-29</p><hr><table><tr><td>Cafe</td><td style='text-align:right'>Bs 5.00</td></tr><tr><td>Te</td><td style='text-align:right'>Bs 3.00</td></tr><tr><td>Jugo</td><td style='text-align:right'>Bs 4.00</td></tr></table><hr><p style='text-align:right'><b>TOTAL: Bs 12.00</b></p><hr><center>Gracias por su compra!</center>\",\"cut\":true}"
echo.
echo.

echo ==========================================
echo   PRUEBAS COMPLETADAS
echo ==========================================
echo.
pause
