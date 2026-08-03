@echo off
title WinSentinel Agent Setup & Installer
color 0B
cls

echo ====================================================================
echo             WINSENTINEL DESKTOP AGENT INSTALLER                     
echo ====================================================================
echo.
echo Welcome! This setup will configure your Server URL and Employee Key
echo to connect this PC to the Monitoring Cloud System.
echo.
echo ====================================================================
echo.

set /p SERVER_URL="Enter Server API URL (Default: http://monitor-cloudd.test/api): "
if "%SERVER_URL%"=="" set SERVER_URL=http://monitor-cloudd.test/api

echo.
set /p EMP_KEY="Enter Employee Key / ID (Default: 03d06c36-3882-4976-905c-864b2975c065): "
if "%EMP_KEY%"=="" set EMP_KEY=03d06c36-3882-4976-905c-864b2975c065

echo.
set /p MACH_NAME="Enter Machine Name (Default: %COMPUTERNAME%): "
if "%MACH_NAME%"=="" set MACH_NAME=%COMPUTERNAME%

echo.
echo --------------------------------------------------------------------
echo CONFIGURATION SUMMARY:
echo Server API URL : %SERVER_URL%
echo Employee Key   : %EMP_KEY%
echo Machine Name   : %MACH_NAME%
echo --------------------------------------------------------------------
echo.

set /p CONFIRM="Proceed with installation? (Y/N): "
if /i not "%CONFIRM%"=="Y" (
    echo Installation cancelled.
    pause
    exit /b
)

set INSTALL_DIR=%APPDATA%\MonitoringAgent
if not exist "%INSTALL_DIR%" mkdir "%INSTALL_DIR%"
if not exist "%INSTALL_DIR%\configs" mkdir "%INSTALL_DIR%\configs"

echo.
echo Creating configuration file at %INSTALL_DIR%\configs\agent.yaml ...

(
echo server:
echo   api_url: "%SERVER_URL%"
echo   heartbeat_interval_sec: 15
echo   employee_id: "%EMP_KEY%"
echo   machine_name: "%MACH_NAME%"
echo.
echo web_server:
echo   enabled: true
echo   host: "0.0.0.0"
echo   port: 8080
echo   auto_open: false
echo.
echo database:
echo   path: "data/agent.db"
echo   max_open_conns: 1
echo   max_idle_conns: 1
echo.
echo logger:
echo   dir: "logs"
echo   level: "info"
echo   max_size_mb: 10
echo   max_backups: 5
echo   max_age_days: 30
echo   compress: true
echo.
echo app_tracker:
echo   enabled: true
echo   poll_interval_sec: 1
echo.
echo browser_tracker:
echo   enabled: true
echo   poll_interval_sec: 1
echo.
echo screenshot:
echo   enabled: true
echo   interval_sec: 60
echo   quality: 75
echo   storage_dir: "data/screenshots"
echo.
echo input:
echo   enabled: true
echo   poll_interval_sec: 1
echo   idle_threshold_sec: 60
echo.
echo sync:
echo   enabled: true
echo   interval_sec: 5
echo   batch_size: 20
) > "%INSTALL_DIR%\configs\agent.yaml"

echo Copying agent.exe executable...
copy /y "%~dp0agent.exe" "%INSTALL_DIR%\agent.exe" >nul

echo Registering automatic Windows startup entry...
reg add "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "MonitoringAgent" /t REG_SZ /d "\"%INSTALL_DIR%\agent.exe\" -config \"%INSTALL_DIR%\configs\agent.yaml\"" /f >nul

echo Stopping existing agent instance (if running)...
taskkill /f /im agent.exe >nul 2>&1

echo Starting silent background monitoring service...
start "" "%INSTALL_DIR%\agent.exe" -config "%INSTALL_DIR%\configs\agent.yaml"

echo.
echo ====================================================================
echo SUCCESS: WinSentinel Agent is fully installed & connected!
echo.
echo  - Server URL   : %SERVER_URL%
echo  - Employee Key : %EMP_KEY%
echo  - Service Status: Running 100%% silently in background
echo  - Auto-start   : Active on Windows boot
echo  - Local Web UI : http://localhost:8080
echo ====================================================================
echo.
pause
