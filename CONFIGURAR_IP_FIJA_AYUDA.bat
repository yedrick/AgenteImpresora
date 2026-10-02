@echo off
echo ==========================================
echo       COLLATECH - AYUDA IP FIJA
echo ==========================================
echo.
echo RECOMENDADO:
echo 1. Usa la URL por nombre de PC que aparece en el panel:
echo    http://NOMBRE-PC:18743/panel
echo.
echo Eso normalmente no cambia aunque cambie la IP.
echo.
echo SI IGUAL QUIERES IP FIJA:
echo Configuralo mejor en el ROUTER como "reserva DHCP" para esta PC.
echo Asi Windows no se rompe y la IP queda siempre igual.
echo.
echo IP actual de esta PC:
ipconfig | findstr /i "IPv4"
echo.
echo Nombre de esta PC:
hostname
echo.
pause
