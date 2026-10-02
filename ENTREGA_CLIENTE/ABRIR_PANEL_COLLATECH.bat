@echo off
setlocal
cd /d "%~dp0"
title CollaTech Agent - Panel

echo ==========================================
echo       COLLATECH AGENT - PANEL
echo ==========================================
echo.

if not exist "CollaTechAgent.exe" (
  echo [ERROR] No existe CollaTechAgent.exe en esta carpeta.
  echo Ejecuta BUILD_SERVER.bat o usa INSTALADOR.exe.
  echo.
  pause
  exit /b 1
)

echo Iniciando agente...
powershell -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -Command "Start-Process -FilePath '%~dp0CollaTechAgent.exe' -WorkingDirectory '%~dp0' -WindowStyle Hidden"

echo Esperando respuesta en http://localhost:18743/health ...
set "OK="
for /l %%i in (1,1,20) do (
  curl.exe -s http://127.0.0.1:18743/health >nul 2>nul
  if not errorlevel 1 (
    set "OK=1"
    goto :ready
  )
  timeout /t 1 /nobreak >nul
)

:ready
if not defined OK (
  echo.
  echo [ERROR] El agente no respondio en el puerto 18743.
  echo.
  echo Ultimos logs:
  if exist "logs" (
    for /f "delims=" %%f in ('dir /b /o-d logs\*.jsonl 2^>nul') do (
      powershell -NoProfile -Command "Get-Content 'logs\%%f' | Select-Object -Last 30"
      goto :fail
    )
  )
  echo No hay logs disponibles.
  :fail
  echo.
  pause
  exit /b 1
)

echo.
echo [OK] Agente funcionando.
echo Panel: http://localhost:18743/panel
echo.
start "" "http://localhost:18743/panel"
echo Si cierras esta ventana, el agente queda corriendo en segundo plano.
echo Para detenerlo usa DETENER_COLLATECH.bat.
echo.
pause
