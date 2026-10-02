$escpos = [byte[]](0x1b,0x40) + [Text.Encoding]::ASCII.GetBytes("COLLATECH RAW`n") + [byte[]](0x1d,0x56,0x00)
$body = @{
  printer = "EPSON"
  data = [Convert]::ToBase64String($escpos)
  base64 = $true
} | ConvertTo-Json

Invoke-RestMethod -Method Post -Uri "http://localhost:18743/api/print/raw" -ContentType "application/json" -Body $body
