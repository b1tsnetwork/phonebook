@echo off
rem Telefonbuch-Symbol entfernen.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-Telefonbuch.ps1" -Uninstall
timeout /t 3 >nul
