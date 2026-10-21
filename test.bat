@echo off
REM Root wrapper: the local tester lives in source_code/.
REM Keeps the historical repository-level test.bat contract.
setlocal
cd /d "%~dp0source_code"
call test.bat %*
exit /b %errorlevel%
