@echo off
setlocal

echo ==================================================
echo        Building and Starting Monitoring Agent UI
echo ==================================================

go build -o ui.exe ./cmd/ui
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Build failed for ui.exe
    pause
    exit /b %ERRORLEVEL%
)

echo Starting UI dashboard...
start ui.exe
