@echo off
REM Starts the standalone ChitChat API Persona Studio & Live Test Chat dashboard (apichat.exe).
REM If build\bin\apichat.exe has not been built yet, automatically builds it first.
setlocal EnableExtensions DisableDelayedExpansion
cd /d "%~dp0"
set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"
if not exist "%~dp0build\bin\apichat.exe" (
    echo [i] build\bin\apichat.exe not found yet - building API Persona Studio now...
    if not exist "%~dp0build\bin" mkdir "%~dp0build\bin"
    set "CGO_ENABLED=0"
    go build -tags desktop,production -ldflags "-w -s -H windowsgui" -o "%~dp0build\bin\apichat.exe" .\cmd\apichat
)
if not exist "%~dp0build\bin\apichat.exe" (
    echo [i] Running install.bat build...
    call "%~dp0install.bat" build
)
if not exist "%~dp0build\bin\apichat.exe" (
    echo [ERROR] Could not build build\bin\apichat.exe. Check build\install.log.
    pause
    exit /b 1
)
start "" "%~dp0build\bin\apichat.exe"
exit /b 0
