# ORBIT 5-Node Local Cluster Orchestrator
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RootDir = Split-Path -Parent $ScriptDir
Set-Location $RootDir

Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host " ORBIT Distributed Key-Value Database — Cluster Launcher  " -ForegroundColor White
Write-Host "==========================================================" -ForegroundColor Cyan

$nodes = @("node-a", "node-b", "node-c", "node-d", "node-e")
$binary = Join-Path $RootDir "bin\orbit.exe"

if (-not (Test-Path $binary)) {
    Write-Host "Building bin\orbit.exe..." -ForegroundColor Yellow
    go build -o bin\orbit.exe ./cmd/orbit
}

# Create log directory
$logDir = Join-Path $RootDir "logs"
if (-not (Test-Path $logDir)) {
    New-Item -ItemType Directory -Path $logDir | Out-Null
}

foreach ($node in $nodes) {
    $cfg = "configs\$node.json"
    $clusterCfg = "configs\cluster.json"
    $stdoutLog = Join-Path $logDir "$node.stdout.log"
    $stderrLog = Join-Path $logDir "$node.stderr.log"
    Write-Host "Starting $node on ports ($cfg)..." -ForegroundColor Green
    Start-Process -FilePath $binary -ArgumentList @("-config", $cfg, "-cluster", $clusterCfg) -WorkingDirectory $RootDir -WindowStyle Hidden -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog
}

Write-Host ""
Write-Host "All 5 nodes launched! Cluster is electing leaders across 3 shards." -ForegroundColor Cyan
Write-Host "Endpoints:" -ForegroundColor White
Write-Host "  - node-a: http://127.0.0.1:2525"
Write-Host "  - node-b: http://127.0.0.1:2526"
Write-Host "  - node-c: http://127.0.0.1:2527"
Write-Host "  - node-d: http://127.0.0.1:2528"
Write-Host "  - node-e: http://127.0.0.1:2529"
Write-Host "Frontend Dashboard: http://localhost:3000/dashboard" -ForegroundColor Green
Write-Host "Run .\scripts\stop-cluster.ps1 to stop all nodes." -ForegroundColor Yellow
