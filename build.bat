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

echo Building svc.exe (boot-time Windows Service)...
go build -ldflags="-s -w" -o svc.exe ./cmd/svc
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build svc.exe
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

echo Copying agent.exe, uninstaller.exe, svc.exe and watchdog.exe to cmd\installer...
copy /Y agent.exe cmd\installer\agent.exe
copy /Y uninstaller.exe cmd\installer\uninstaller.exe
copy /Y svc.exe cmd\installer\svc.exe
copy /Y watchdog.exe cmd\installer\watchdog.exe

echo Building Installer.exe...
go build -ldflags="-s -w" -o Installer.exe ./cmd/installer
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to build Installer.exe
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
echo Copying binaries to dist and web public/downloads...
if not exist dist mkdir dist
copy /Y agent.exe dist\agent.exe
copy /Y uninstaller.exe dist\uninstaller.exe
copy /Y Installer.exe dist\Installer.exe
copy /Y watchdog.exe dist\watchdog.exe
copy /Y svc.exe dist\svc.exe
copy /Y ui.exe dist\ui.exe

echo.
echo Writing agent-manifest.json (the version agent.exe reports + its SHA-256)...
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\write-agent-manifest.ps1 -Agent agent.exe -Out dist\agent-manifest.json
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to write agent-manifest.json
    pause
    exit /b %ERRORLEVEL%
)

rem The web server announces updates from agent-manifest.json, so it always travels with agent.exe.
if exist ..\..\web\monitor-cloudd\public\downloads (
    copy /Y agent.exe ..\..\web\monitor-cloudd\public\downloads\agent.exe
    copy /Y dist\agent-manifest.json ..\..\web\monitor-cloudd\public\downloads\agent-manifest.json
    copy /Y uninstaller.exe ..\..\web\monitor-cloudd\public\downloads\uninstaller.exe
    copy /Y Installer.exe ..\..\web\monitor-cloudd\public\downloads\Installer.exe
)
if exist C:\Projects\web\monitor-cloudd\public\downloads (
    copy /Y agent.exe C:\Projects\web\monitor-cloudd\public\downloads\agent.exe
    copy /Y dist\agent-manifest.json C:\Projects\web\monitor-cloudd\public\downloads\agent-manifest.json
    copy /Y uninstaller.exe C:\Projects\web\monitor-cloudd\public\downloads\uninstaller.exe
    copy /Y Installer.exe C:\Projects\web\monitor-cloudd\public\downloads\Installer.exe
)
