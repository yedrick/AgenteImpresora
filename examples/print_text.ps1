$body = @{
  printer = "EPSON"
  text = "COLLATECH`nGracias por su compra"
  cut = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:18743/api/print/text" -ContentType "application/json" -Body $body
