@echo off
cd /d "%~dp0"
echo ==================================================
echo     Fetching SQLite Database Contents...          
echo ==================================================
go run ./cmd/db_viewer
pause
