@echo off
cd /d "%~dp0"
echo ==========================================
echo       BUILD COLLATECH AGENT SERVER
echo ==========================================
echo.
set GOCACHE=%CD%\.gocache
set GOOS=windows
set GOARCH=amd64
go version
if errorlevel 1 (
  echo.
  echo ERROR: Go no esta instalado o no esta en PATH.
  echo Instala Go 1.22+ y vuelve a ejecutar.
  pause
  exit /b 1
)
echo.
echo Compilando CollaTechAgent.exe...
go build -buildvcs=false -ldflags "-H windowsgui -s -w" -o CollaTechAgent.exe ./cmd/server
if errorlevel 1 (
  echo.
  echo ERROR: Fallo el build del servidor.
  pause
  exit /b 1
)
echo.
echo OK: CollaTechAgent.exe generado.
pause
