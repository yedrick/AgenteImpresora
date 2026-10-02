$body = @{
  printer = "EPSON"
  title = "COLLATECH"
  qr = "https://kollatek.com"
  cut = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:18743/api/print/ticket" -ContentType "application/json" -Body $body
