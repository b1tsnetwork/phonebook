@echo off
rem Telefonbuch-Symbol fuer den angemeldeten Benutzer installieren (keine Administratorrechte noetig).
setlocal
set "TB_URL=__TELEFONBUCH_URL__"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Install-Telefonbuch.ps1" -Url "%TB_URL%"
if errorlevel 1 (
  echo.
  echo Installation fehlgeschlagen.
  pause
  exit /b 1
)
timeout /t 4 >nul
