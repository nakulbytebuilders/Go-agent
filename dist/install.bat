@echo off
title Installing Monitoring Agent Background Service
echo ==================================================
echo   INSTALLING SILENT BACKGROUND MONITORING SERVICE  
echo ==================================================
echo.

set INSTALL_DIR=%APPDATA%\MonitoringAgent
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"

echo Copying agent executable and configuration...
copy /y "%~dp0agent.exe" "%INSTALL_DIR%\agent.exe" >nul
if exist "%~dp0configs\agent.yaml" (
    if not exist "%INSTALL_DIR%\configs" mkdir "%INSTALL_DIR%\configs"
    copy /y "%~dp0configs\agent.yaml" "%INSTALL_DIR%\configs\agent.yaml" >nul
)

echo Registering automatic Windows startup entry...
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "MonitoringAgent" /t REG_SZ /d "\"%INSTALL_DIR%\agent.exe\"" /f >nul

echo Stopping any existing instance...
taskkill /f /im agent.exe >nul 2>&1

echo Launching silent background service...
start "" "%INSTALL_DIR%\agent.exe"

echo.
echo ==================================================
echo SUCCESS: Monitoring Agent is installed!
echo - Running 100%% silently in the background (no console window).
echo - Will auto-start automatically whenever PC reboots.
echo - Web Dashboard available at: http://localhost:8080
echo ==================================================
timeout /t 5 >nul
