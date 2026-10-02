@echo off
setlocal
cd /d "%~dp0"
title CollaTech Agent - Cambiar a puerto 8080

echo ==========================================
echo   COLLATECH AGENT - CAMBIAR A PUERTO 8080
echo ==========================================
echo.
echo IMPORTANTE: ejecuta este archivo como Administrador.
echo.

net session >nul 2>nul
if not "%errorlevel%"=="0" (
  echo [ERROR] Este archivo requiere permisos de Administrador.
  echo Clic derecho ^> Ejecutar como administrador.
  echo.
  pause
  exit /b 1
)

set "CFG=%~dp0configs\config.json"
if not exist "%CFG%" (
  if exist "%ProgramFiles%\CollaTech Agent\configs\config.json" (
    set "CFG=%ProgramFiles%\CollaTech Agent\configs\config.json"
  )
)

if not exist "%CFG%" (
  echo [ERROR] No encontre configs\config.json.
  echo Ejecuta este archivo desde la carpeta de CollaTech o instala primero el agente.
  pause
  exit /b 1
)

echo [1] Deteniendo CollaTech Agent si esta abierto...
taskkill /IM CollaTechAgent.exe /F >nul 2>nul

echo [2] Cambiando config a host 0.0.0.0 y puerto 8080...
echo     Config: %CFG%
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$path='%CFG%';" ^
  "$cfg=Get-Content -Raw -LiteralPath $path | ConvertFrom-Json;" ^
  "if ($null -eq $cfg.PSObject.Properties['host']) { $cfg | Add-Member -NotePropertyName host -NotePropertyValue '0.0.0.0' } else { $cfg.host='0.0.0.0' };" ^
  "if ($null -eq $cfg.PSObject.Properties['port']) { $cfg | Add-Member -NotePropertyName port -NotePropertyValue 8080 } else { $cfg.port=8080 };" ^
  "if ($null -eq $cfg.PSObject.Properties['allow_remote']) { $cfg | Add-Member -NotePropertyName allow_remote -NotePropertyValue $true } else { $cfg.allow_remote=$true };" ^
  "if ($null -ne $cfg.PSObject.Properties['tls']) { $cfg.tls.enabled=$false };" ^
  "$cfg | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $path -Encoding UTF8"
if not "%errorlevel%"=="0" (
  echo [ERROR] No se pudo actualizar la configuracion.
  pause
  exit /b 1
)

set "CFG_PARENT=%~dp0..\configs\config.json"
if exist "%CFG_PARENT%" (
  echo     Config adicional: %CFG_PARENT%
  powershell -NoProfile -ExecutionPolicy Bypass -Command ^
    "$path='%CFG_PARENT%';" ^
    "$cfg=Get-Content -Raw -LiteralPath $path | ConvertFrom-Json;" ^
    "if ($null -eq $cfg.PSObject.Properties['host']) { $cfg | Add-Member -NotePropertyName host -NotePropertyValue '0.0.0.0' } else { $cfg.host='0.0.0.0' };" ^
    "if ($null -eq $cfg.PSObject.Properties['port']) { $cfg | Add-Member -NotePropertyName port -NotePropertyValue 8080 } else { $cfg.port=8080 };" ^
    "if ($null -eq $cfg.PSObject.Properties['allow_remote']) { $cfg | Add-Member -NotePropertyName allow_remote -NotePropertyValue $true } else { $cfg.allow_remote=$true };" ^
    "if ($null -ne $cfg.PSObject.Properties['tls']) { $cfg.tls.enabled=$false };" ^
    "$cfg | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $path -Encoding UTF8"
)

echo [3] Abriendo puerto 8080 en Firewall y cerrando el 18743...
netsh advfirewall firewall delete rule name="CollaTech Agent 8080" >nul 2>nul
netsh advfirewall firewall add rule name="CollaTech Agent 8080" dir=in action=allow protocol=TCP localport=8080 profile=private,domain

REM Antes el 18743 se quedaba abierto para siempre: el equipo acababa con dos
REM puertos de entrada aunque el agente solo escuchara en uno.
netsh advfirewall firewall delete rule name="GOServer18743" >nul 2>nul
netsh advfirewall firewall delete rule name="CollaTech Agent 18743" >nul 2>nul

if exist "%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" (
  netsh advfirewall firewall delete rule name="CollaTech Agent App" >nul 2>nul
  netsh advfirewall firewall add rule name="CollaTech Agent App" dir=in action=allow program="%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" enable=yes profile=private,domain
)

echo.
echo [4] Volviendo a arrancar el agente...
REM Antes el script mataba el proceso y dejaba al cliente sin imprimir hasta
REM que arrancara el agente a mano.
sc query CollaTechAgent >nul 2>nul
if %errorlevel% equ 0 (
  sc start CollaTechAgent >nul 2>nul
) else (
  if exist "%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" (
    start "" /D "%ProgramFiles%\CollaTech Agent" "%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe"
  ) else (
    if exist "%~dp0CollaTechAgent.exe" start "" /D "%~dp0" "%~dp0CollaTechAgent.exe"
  )
)

echo     Esperando a que responda...
set "LISTO="
for /l %%i in (1,1,20) do (
  if not defined LISTO (
    curl.exe -s -o nul http://127.0.0.1:8080/health && set "LISTO=1"
    if not defined LISTO timeout /t 1 /nobreak >nul
  )
)

echo.
if defined LISTO (
  echo [OK] Configurado en puerto 8080 y agente en marcha.
) else (
  echo [AVISO] Configurado en puerto 8080, pero el agente no respondio.
  echo         Abre CollaTechAgent.exe a mano y revisa la carpeta logs\.
)
echo.
echo Prueba desde esta PC:
echo   http://127.0.0.1:8080/health
echo.
echo NOTA: para volver al puerto 18743, edita configs\config.json y vuelve a
echo       ejecutar ABRIR_FIREWALL_LAN.bat como administrador.
echo.
echo Desde celular prueba:
for /f "tokens=2 delims=:" %%a in ('ipconfig ^| findstr /i "IPv4"') do (
  set "IP=%%a"
  setlocal enabledelayedexpansion
  set "IP=!IP: =!"
  echo   http://!IP!:8080/health
  echo   http://!IP!:8080/panel
  endlocal
)
echo.
pause
