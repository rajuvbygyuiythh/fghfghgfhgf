@echo off
setlocal
cd /d "%~dp0"
echo [1/2] Running Step-by-Step Deep Project ^& 102-Site Diagnostics...
node source_code\scripts\deep-debug.cjs
if errorlevel 1 exit /b 1
echo.
echo [2/2] Running Automated UI ^& Contract Verification Suite...
cd /d "%~dp0source_code"
node --test tests\*.test.cjs
endlocal
