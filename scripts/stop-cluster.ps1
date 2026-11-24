# Stop all ORBIT cluster processes
Write-Host "Stopping all ORBIT node processes..." -ForegroundColor Yellow
Get-Process -Name "orbit" -ErrorAction SilentlyContinue | Stop-Process -Force
Write-Host "All ORBIT cluster nodes stopped." -ForegroundColor Green
