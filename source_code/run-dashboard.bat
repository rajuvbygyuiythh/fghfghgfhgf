@echo off
REM Starts the built ChitChat Bot dashboard. If build\bin\chitchatui.exe has
REM not been built yet, automatically runs install.bat build first.
setlocal EnableExtensions DisableDelayedExpansion
cd /d "%~dp0"
if not exist "%~dp0build\bin\chitchatui.exe" (
    echo [i] build\bin\chitchatui.exe not found yet - building dashboard now...
    call "%~dp0install.bat" build
)
if not exist "%~dp0build\bin\chitchatui.exe" (
    echo [ERROR] Could not build build\bin\chitchatui.exe. Check build\install.log.
    pause
    exit /b 1
)
start "" "%~dp0build\bin\chitchatui.exe"
exit /b 0
