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

echo [6] Intentando reforzar reglas firewall (requiere administrador)
netsh advfirewall firewall delete rule name="CollaTech Agent 18743" >nul 2>nul
netsh advfirewall firewall delete rule name="CollaTech Agent App" >nul 2>nul
netsh advfirewall firewall add rule name="CollaTech Agent 18743" dir=in action=allow protocol=TCP localport=18743 profile=any
if exist "%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" (
  netsh advfirewall firewall add rule name="CollaTech Agent App" dir=in action=allow program="%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" enable=yes profile=any
)
if exist "%~dp0CollaTechAgent.exe" (
  netsh advfirewall firewall add rule name="CollaTech Agent App" dir=in action=allow program="%~dp0CollaTechAgent.exe" enable=yes profile=any
)
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
