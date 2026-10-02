@echo off
setlocal
title CollaTech Agent - Probar puerto desde otra PC

set "IP=192.168.1.250"
set "PORT=18743"

echo ==========================================
echo   COLLATECH AGENT - PRUEBA DESDE CLIENTE
echo ==========================================
echo.
echo Esta prueba debe ejecutarse en OTRA PC conectada a la misma red WiFi/LAN.
echo IP del servidor CollaTech: %IP%
echo Puerto: %PORT%
echo.

echo [1] Ping a la PC servidor
ping -n 4 %IP%
echo.

echo [2] Prueba del puerto TCP
powershell -NoProfile -ExecutionPolicy Bypass -Command "Test-NetConnection %IP% -Port %PORT%"
echo.

echo [3] Prueba HTTP /health
curl.exe -s -S http://%IP%:%PORT%/health
echo.
echo.

echo Resultado esperado:
echo - TcpTestSucceeded: True
echo - Respuesta: {"ok":true,"message":"healthy"}
echo.
echo Si el ping funciona pero TcpTestSucceeded es False:
echo - El firewall de Windows/antivirus bloquea el puerto, o
echo - El router tiene aislamiento de clientes/AP Isolation.
echo.
pause
