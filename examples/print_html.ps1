$body = @{
  printer = "EPSON"
  html = "<html><body><h1>CollaTech</h1><p>Factura demo</p><p>Total: Bs 120.00</p></body></html>"
  width = 576
  cut = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:18743/api/print/html" -ContentType "application/json" -Body $body
