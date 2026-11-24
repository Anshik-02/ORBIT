@echo off
echo ==========================================================
echo  ORBIT Distributed Key-Value Database -- Cluster Launcher 
echo ==========================================================
cd /d "%~dp0\.."

if not exist "bin\orbit.exe" (
    echo Building bin\orbit.exe...
    go build -o bin\orbit.exe .\cmd\orbit
)

echo Starting node-a (port 2525)...
start "ORBIT node-a" /min bin\orbit.exe -config configs/node-a.json -cluster configs/cluster.json

echo Starting node-b (port 2526)...
start "ORBIT node-b" /min bin\orbit.exe -config configs/node-b.json -cluster configs/cluster.json

echo Starting node-c (port 2527)...
start "ORBIT node-c" /min bin\orbit.exe -config configs/node-c.json -cluster configs/cluster.json

echo Starting node-d (port 2528)...
start "ORBIT node-d" /min bin\orbit.exe -config configs/node-d.json -cluster configs/cluster.json

echo Starting node-e (port 2529)...
start "ORBIT node-e" /min bin\orbit.exe -config configs/node-e.json -cluster configs/cluster.json

echo.
echo All 5 nodes launched in background!
echo Endpoints:
echo   - node-a: http://127.0.0.1:2525
echo   - node-b: http://127.0.0.1:2526
echo   - node-c: http://127.0.0.1:2527
echo   - node-d: http://127.0.0.1:2528
echo   - node-e: http://127.0.0.1:2529
echo Frontend Dashboard: http://localhost:3000/dashboard
echo Run scripts\stop-cluster.bat to stop all nodes.
