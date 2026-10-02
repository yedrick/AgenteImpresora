@echo off
cd /d "%~dp0"
echo ==========================================
echo       BUILD COLLATECH INSTALLER
echo ==========================================
echo.
set GOCACHE=%CD%\.gocache
set GOOS=windows
set GOARCH=amd64
go version
if errorlevel 1 (
  echo.
  echo ERROR: Go no esta instalado o no esta en PATH.
  pause
  exit /b 1
)
echo.
echo Generando binario embebido para instalador...
go build -buildvcs=false -ldflags "-H windowsgui -s -w" -o cmd\installer\CollaTechAgent.bin ./cmd/server
if errorlevel 1 (
  echo ERROR: No se pudo compilar el agente embebido.
  pause
  exit /b 1
)
echo.
echo Compilando INSTALADOR.exe...
go build -buildvcs=false -ldflags "-H windowsgui -s -w" -o INSTALADOR.exe ./cmd/installer
if errorlevel 1 (
  echo ERROR: Fallo el build del instalador.
  pause
  exit /b 1
)
echo.
echo OK: INSTALADOR.exe generado.
pause
