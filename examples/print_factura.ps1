$html = @"
<html>
  <body>
    <h1>CollaTech</h1>
    <p>Cliente: Juan Perez</p>
    <p>Producto A - Bs 50</p>
    <p>Producto B - Bs 70</p>
    <h2>Total: Bs 120</h2>
  </body>
</html>
"@

$body = @{
  printer = "EPSON"
  html = $html
  width = 576
  cut = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:18743/api/print/html" -ContentType "application/json" -Body $body
