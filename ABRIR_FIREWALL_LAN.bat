@echo off
REM Las reglas se crean solo en los perfiles Privado y Dominio. Con
REM "profile=any" el puerto quedaba abierto tambien en redes Publicas (WiFi de
REM cafeteria, aeropuerto), donde no hay nada que imprimir.
echo ==========================================
echo   PERMITIR COLLATECH AGENT EN RED LOCAL
echo ==========================================
echo.
echo Este comando necesita permisos de administrador.
echo Puerto: 18743 TCP
echo.
netsh advfirewall firewall delete rule name="GOServer18743" >nul 2>nul
netsh advfirewall firewall delete rule name="CollaTech Agent 18743" >nul 2>nul
netsh advfirewall firewall delete rule name="CollaTech Agent App" >nul 2>nul
netsh advfirewall firewall add rule name="GOServer18743" dir=in action=allow protocol=TCP localport=18743 profile=private,domain
if errorlevel 1 (
  echo.
  echo ERROR: No se pudo abrir el firewall.
  echo Ejecuta este archivo con clic derecho: Ejecutar como administrador.
) else (
  echo.
  echo OK: Firewall abierto para la red local.
)
netsh advfirewall firewall add rule name="CollaTech Agent 18743" dir=in action=allow protocol=TCP localport=18743 profile=private,domain >nul 2>nul
if exist "%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" (
  netsh advfirewall firewall add rule name="CollaTech Agent App" dir=in action=allow program="%ProgramFiles%\CollaTech Agent\CollaTechAgent.exe" enable=yes profile=private,domain
)
if exist "%~dp0CollaTechAgent.exe" (
  netsh advfirewall firewall add rule name="CollaTech Agent App" dir=in action=allow program="%~dp0CollaTechAgent.exe" enable=yes profile=private,domain
)
pause
