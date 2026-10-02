@echo off
echo ==========================================
echo   GENERAR CERTIFICADO SSL PARA COLLATECH
echo ==========================================
echo.

if not exist "certs" mkdir certs

echo Generando certificado autofirmado...
echo.

REM OpenSSL debe estar instalado
REM Si no esta, descargar de: https://slproweb.com/products/Win32OpenSSL.html

openssl req -x509 -newkey rsa:2048 -keyout certs\key.pem -out certs\cert.pem -days 365 -nodes -subj "/CN=localhost"

echo.
echo ==========================================
echo   CERTIFICADO GENERADO
echo ==========================================
echo.
echo Archivos:
echo   certs\cert.pem
echo   certs\key.pem
echo.
echo Para activar HTTPS, edita configs\config.json:
echo   "tls": {
echo     "enabled": true,
echo     "cert_file": "certs/cert.pem",
echo     "key_file": "certs/key.pem"
echo   }
echo.
pause
