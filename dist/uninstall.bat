@echo off
title Uninstalling WinSentinel Monitoring Agent
echo ==================================================
echo   UNINSTALLING WINSENTINEL MONITORING AGENT
echo ==================================================
echo.

if exist "%APPDATA%\MonitoringAgent\uninstaller.exe" (
    echo Running uninstaller.exe...
    "%APPDATA%\MonitoringAgent\uninstaller.exe"
) else if exist "uninstaller.exe" (
    echo Running local uninstaller.exe...
    "uninstaller.exe"
) else (
    echo Stopping processes...
    taskkill /f /t /im watchdog.exe >nul 2>&1
    taskkill /f /t /im agent.exe >nul 2>&1

    echo Removing scheduled tasks...
    schtasks /Delete /TN "\Microsoft\Windows\Hotpatch\Monitoring" /F >nul 2>&1
    schtasks /Delete /TN "Monitoring" /F >nul 2>&1

    echo Removing registry entries...
    reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "WinSentinelAgent" /f >nul 2>&1
    reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Run" /v "WinSentinelWatchdog" /f >nul 2>&1
    reg delete "HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent" /f >nul 2>&1
    reg delete "HKLM\Software\Microsoft\Windows\CurrentVersion\Uninstall\WinSentinelAgent" /f >nul 2>&1

    echo Purging installation directory...
    rmdir /s /q "%APPDATA%\MonitoringAgent" >nul 2>&1
)

echo.
echo ==================================================
echo SUCCESS: WinSentinel Monitoring Agent has been completely uninstalled.
echo ==================================================
pause

