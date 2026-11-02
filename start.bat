@echo off
chcp 65001 >nul
powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0scripts\dev-up.ps1" %*
echo.
pause
