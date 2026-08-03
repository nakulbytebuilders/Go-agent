@echo off
title Uninstalling Monitoring Agent
echo ==================================================
echo   UNINSTALLING MONITORING AGENT BACKGROUND SERVICE
echo ==================================================
echo.

echo Stopping silent agent process...
taskkill /f /im agent.exe >nul 2>&1

echo Removing automatic Windows startup entry...
reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "MonitoringAgent" /f >nul 2>&1

echo.
echo ==================================================
echo SUCCESS: Monitoring Agent has been uninstalled.
echo ==================================================
pause
