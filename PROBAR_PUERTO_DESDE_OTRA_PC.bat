@echo off
setlocal
title CollaTech Agent - Probar puerto desde otra PC

REM La IP se pregunta: estaba cableada a 192.168.1.250, asi que en cualquier
REM instalacion con otra IP el script probaba un equipo ajeno y daba un falso
REM negativo.
set "PORT=18743"
set /p "IP=IP de la PC donde corre CollaTech Agent (ej. 192.168.1.50): "
if "%IP%"=="" (
  echo No se indico ninguna IP. Mirala en el panel del agente, pestana Estado.
  pause
  exit /b 1
)

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
