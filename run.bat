@echo off
cd /d "%~dp0"
echo ==================================================
echo        Building Cross-Platform Monitoring Agent   
echo ==================================================

go build -o agent.exe ./cmd/agent
if %ERRORLEVEL% NEQ 0 (
    echo.
    echo [ERROR] Build failed! Check errors above.
    pause
    exit /b %ERRORLEVEL%
)

echo Build successful.
echo.
echo ==================================================
echo        Starting Monitoring Agent                  
echo ==================================================
agent.exe -config configs/agent.yaml %*

pause
