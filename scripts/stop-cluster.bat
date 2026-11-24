@echo off
echo Stopping all ORBIT node processes...
taskkill /F /IM orbit.exe /T 2>nul
echo All ORBIT cluster nodes stopped.
