@echo off
echo Deteniendo el servicio CollaTech Agent...
echo (Este comando necesita permisos de administrador)
sc stop CollaTechAgent
echo.
echo Si necesitas que deje de arrancar solo al encender la PC:
echo   sc config CollaTechAgent start= demand
pause
