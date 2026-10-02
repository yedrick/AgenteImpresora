$body = @{
  printer = "EPSON"
  title = "BARCODE"
  barcode = "123456789"
  cut = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:18743/api/print/ticket" -ContentType "application/json" -Body $body
