# prueba_impresion.ps1
# Script de prueba para CollaTech Agent
# Ejecutar: .\prueba_impresion.ps1

$baseUrl = "http://localhost:18743"

Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  PRUEBA DE IMPRESION - CollaTech Agent" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""

# Verificar conexion
Write-Host "Verificando conexion..." -ForegroundColor Yellow
try {
    $health = Invoke-RestMethod -Uri "$baseUrl/health" -Method Get
    Write-Host "Agente conectado: $($health.message)" -ForegroundColor Green
} catch {
    Write-Host "ERROR: No se puede conectar al agente" -ForegroundColor Red
    Write-Host "Asegurate de que CollaTechAgent.exe este corriendo" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "----------------------------------------" -ForegroundColor Gray

# 1. Ticket estructurado
Write-Host "`n[1] Imprimiendo TICKET..." -ForegroundColor Yellow
$ticket = @{
    printer = "Impresora1"
    title = "MI TIENDA C.A."
    lines = @(
        @{ text = "Factura #001234"; align = "center"; bold = $true }
        @{ text = "Fecha: 2026-05-29"; align = "left" }
        @{ text = "--------------------------------"; align = "left" }
        @{ text = "Cafe ............ Bs 5.00"; align = "left" }
        @{ text = "Te .............. Bs 3.00"; align = "left" }
        @{ text = "Jugo ............ Bs 4.00"; align = "left" }
        @{ text = "--------------------------------"; align = "left" }
        @{ text = "TOTAL: Bs 12.00"; align = "right"; bold = $true }
    )
    cut = $true
} | ConvertTo-Json -Depth 3

try {
    $result = Invoke-RestMethod -Uri "$baseUrl/api/print/ticket" -Method Post -Body $ticket -ContentType "application/json"
    Write-Host "Ticket enviado: $($result.message)" -ForegroundColor Green
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
}

Start-Sleep -Seconds 1

# 2. Template recibo
Write-Host "`n[2] Imprimiendo RECIBO (template)..." -ForegroundColor Yellow
$recibo = @{
    printer = "Impresora1"
    template = "recibo"
    data = @{
        empresa = "Mi Tienda C.A."
        cliente = "Maria Lopez"
        total = "Bs 250.00"
        mensaje = "Gracias por su compra!"
    }
    cut = $true
} | ConvertTo-Json -Depth 3

try {
    $result = Invoke-RestMethod -Uri "$baseUrl/api/print/template" -Method Post -Body $recibo -ContentType "application/json"
    Write-Host "Recibo enviado: $($result.message)" -ForegroundColor Green
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
}

Start-Sleep -Seconds 1

# 3. HTML nativo
Write-Host "`n[3] Imprimiendo HTML nativo..." -ForegroundColor Yellow
$html = @"
<center><b>COMPROBANTE DE PAGO</b></center>
<hr>
<p>Factura: #001234</p>
<p>Cliente: Juan Perez</p>
<p>Fecha: 2026-05-29</p>
<hr>
<table>
<tr><td>Cafe</td><td style="text-align:right">Bs 5.00</td></tr>
<tr><td>Te</td><td style="text-align:right">Bs 3.00</td></tr>
<tr><td>Jugo</td><td style="text-align:right">Bs 4.00</td></tr>
</table>
<hr>
<p style="text-align:right"><b>TOTAL: Bs 12.00</b></p>
<hr>
<center>Gracias por su compra!</center>
"@

$printHtml = @{
    printer = "Impresora1"
    html = $html
    cut = $true
} | ConvertTo-Json -Depth 3

try {
    $result = Invoke-RestMethod -Uri "$baseUrl/api/print/html" -Method Post -Body $printHtml -ContentType "application/json"
    Write-Host "HTML enviado: $($result.message)" -ForegroundColor Green
} catch {
    Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
}

Write-Host ""
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host "  PRUEBAS COMPLETADAS" -ForegroundColor Cyan
Write-Host "==========================================" -ForegroundColor Cyan
Write-Host ""
