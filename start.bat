@echo off
cd /d "%~dp0"
title Monitoring Agent Launcher
:menu
cls
echo ==================================================
echo       CROSS-PLATFORM MONITORING AGENT LAUNCHER    
echo ==================================================
echo.
echo   [1] Start Agent Background Service (Web UI: http://localhost:8080)
echo   [2] Open Browser Dashboard (http://localhost:8080)
echo   [3] Build All Binaries (agent.exe, watchdog.exe)
echo   [4] View SQLite Database Data
echo   [5] Run All Unit Tests
echo   [6] Exit
echo.
echo ==================================================
set /p CHOICE="Select an option (1-6): "

if "%CHOICE%"=="1" goto run_agent
if "%CHOICE%"=="2" goto open_browser
if "%CHOICE%"=="3" goto build_all
if "%CHOICE%"=="4" goto view_db
if "%CHOICE%"=="5" goto run_tests
if "%CHOICE%"=="6" goto end

echo Invalid selection!
timeout /t 2 >nul
goto menu

:run_agent
cls
echo Starting Agent Service and Web Server...
go run ./cmd/agent
pause
goto menu

:open_browser
cls
echo Opening Browser Dashboard at http://localhost:8080 ...
start http://localhost:8080
timeout /t 2 >nul
goto menu

:build_all
cls
echo Building binaries...
go build -o agent.exe ./cmd/agent
go build -o watchdog.exe ./cmd/watchdog
echo Build completed successfully!
pause
goto menu

:view_db
cls
go run ./cmd/db_viewer
pause
goto menu

:run_tests
cls
go test -v ./...
pause
goto menu

:end
exit
