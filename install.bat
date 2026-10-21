@echo off
REM Root wrapper: all build code lives in source_code\.
REM This proxy keeps the real installer/build logic in one place.
setlocal
cd /d "%~dp0source_code"
call install.bat %*
exit /b %errorlevel%
