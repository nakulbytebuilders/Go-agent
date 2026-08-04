@echo off
echo ==================================================
echo        Building Agent Executables                 
echo ==================================================

echo Building agent.exe...
go build -ldflags="-s -w -H=windowsgui" -o agent.exe ./cmd/agent
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build agent.exe
    pause
    exit /b %ERRORLEVEL%
)

echo Building uninstaller.exe...
go build -ldflags="-s -w -H=windowsgui" -o uninstaller.exe ./cmd/uninstaller
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build uninstaller.exe
    pause
    exit /b %ERRORLEVEL%
)

echo Copying agent.exe and uninstaller.exe to cmd\installer...
copy /Y agent.exe cmd\installer\agent.exe
copy /Y uninstaller.exe cmd\installer\uninstaller.exe

echo Building Installer.exe...
go build -ldflags="-s -w" -o Installer.exe ./cmd/installer
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build Installer.exe
    pause
    exit /b %ERRORLEVEL%
)

echo Building watchdog.exe...
go build -ldflags="-s -w -H=windowsgui" -o watchdog.exe ./cmd/watchdog
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build watchdog.exe
    pause
    exit /b %ERRORLEVEL%
)

echo Building ui.exe...
go build -ldflags="-s -w -H=windowsgui" -o ui.exe ./cmd/ui
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build ui.exe
    pause
    exit /b %ERRORLEVEL%
)

echo.
echo Build completed successfully!
pause
