@echo off
setlocal
cd /d "%~dp0"
title CollaTech Agent - Diagnostico de Red

echo ==========================================
echo   COLLATECH AGENT - DIAGNOSTICO DE RED
echo ==========================================
echo.

echo [1] IP de esta PC
ipconfig | findstr /i "IPv4 Puerta"
echo.

echo [2] Agente escuchando en puerto 18743
netstat -ano | findstr ":18743"
echo.

echo [3] Prueba local por localhost
curl.exe -s -S http://127.0.0.1:18743/health
echo.
echo.

echo [4] Prueba local por IP LAN detectada
for /f "tokens=2 delims=:" %%a in ('ipconfig ^| findstr /i "IPv4"') do (
  set "IP=%%a"
  setlocal enabledelayedexpansion
  set "IP=!IP: =!"
  echo Probando http://!IP!:18743/health
  curl.exe -s -S http://!IP!:18743/health
  echo.
  endlocal
)
echo.

echo [5] Regla firewall actual
netsh advfirewall firewall show rule name="CollaTech Agent 18743"
echo.
netsh advfirewall firewall show rule name="CollaTech Agent App"
echo.

REM Un script de diagnostico no debe modificar el sistema. Antes borraba y
REM recreaba las reglas del firewall aqui: si se ejecutaba sin administrador,
REM el delete funcionaba y el add fallaba, dejando al cliente peor que antes.
echo [6] Si falta alguna regla, ejecuta ABRIR_FIREWALL_LAN.bat como administrador
echo.

echo [7] URLs para probar desde celular
echo Abre primero /health, no /panel:
for /f "tokens=2 delims=:" %%a in ('ipconfig ^| findstr /i "IPv4"') do (
  set "IP=%%a"
  setlocal enabledelayedexpansion
  set "IP=!IP: =!"
  echo http://!IP!:18743/health
  echo http://!IP!:18743/panel
  endlocal
)
echo.

echo Si desde esta PC funciona y desde celular da TIMEOUT:
echo - El celular puede estar en red invitados.
echo - El router puede tener AP Isolation / Client Isolation.
echo - El celular puede estar usando datos moviles, VPN o DNS privado.
echo - Un antivirus/firewall externo puede estar bloqueando entrada.
echo.
pause
