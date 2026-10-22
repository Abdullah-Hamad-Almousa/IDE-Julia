@echo off
setlocal
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0run-jide.ps1" %*
exit /b %ERRORLEVEL%
