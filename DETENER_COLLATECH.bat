@echo off
title CollaTech Agent - Detener
echo Deteniendo CollaTech Agent...
echo (Detener el servicio necesita permisos de administrador)
echo.

sc stop CollaTechAgent >nul 2>nul
if %errorlevel% equ 0 (
  echo Servicio detenido.
) else (
  echo El servicio no estaba corriendo o no se pudo detener.
)

REM Si el agente se lanzo con ABRIR_PANEL_COLLATECH.bat no es un servicio, asi
REM que "sc stop" no lo para. Antes se quedaba corriendo sin que nadie avisara.
tasklist /FI "IMAGENAME eq CollaTechAgent.exe" 2>nul | find /i "CollaTechAgent.exe" >nul
if %errorlevel% equ 0 (
  echo Quedaba un proceso suelto: cerrandolo...
  taskkill /IM CollaTechAgent.exe /F >nul 2>nul
)

echo.
echo Comprobacion final:
tasklist /FI "IMAGENAME eq CollaTechAgent.exe" 2>nul | find /i "CollaTechAgent.exe" >nul
if %errorlevel% equ 0 (
  echo   AVISO: sigue habiendo un proceso CollaTechAgent.exe.
) else (
  echo   CollaTech Agent detenido.
)

echo.
echo Si necesitas que deje de arrancar solo al encender la PC:
echo   sc config CollaTechAgent start= demand
pause
