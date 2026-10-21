@echo off
REM ============================================================================
REM  ChitChat Bot - local message test (console)
REM
REM  Chat with the bot as a stranger, using the SAME four Message options and
REM  the SAME saved dashboard settings. Nothing is sent to the real app, no
REM  browser is opened and no bot state is written.
REM
REM  Usage:  test.bat        menu with the 4 dashboard Message options
REM          test.bat 1      Single Sequence
REM          test.bat 2      Folder Sequences
REM          test.bat 3      EVAChat Flow
REM          test.bat 4      API Chat
REM
REM  Close the dashboard before testing: it keeps the settings database open.
REM ============================================================================
setlocal EnableExtensions
title ChitChat Bot - Local Message Test
cd /d "%~dp0"

REM UTF-8 console so any non-ASCII line in a sequence/pool shows correctly
chcp 65001 >nul

set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"
set "BIN=%CD%\build\bin\chitchattest.exe"

echo.
echo  ==================================================
echo   ChitChat Bot - local message test
echo   project: %CD%
echo  ==================================================
echo.

if not exist "%CD%\go.mod" (
    echo  [ERROR] go.mod not found. Run test.bat from the project folder.
    echo.
    pause
    exit /b 1
)

where go >nul 2>nul
if errorlevel 1 (
    if exist "%BIN%" (
        echo  [OK] Go was not found, starting the previously built tester.
        goto :run
    )
    echo  [ERROR] Go was not found.
    echo  Run install.bat once - it installs Go 1.25+ and builds the dashboard.
    echo.
    pause
    exit /b 1
)

if not exist "%CD%\build\bin" mkdir "%CD%\build\bin"

echo  [..] building the local tester (first run can take a few seconds)...
go build -o "%BIN%" ./cmd/chitchattest
if errorlevel 1 (
    echo.
    echo  [ERROR] Build failed. The Go errors are above.
    echo  Send the text of this window if you want it fixed.
    echo.
    pause
    exit /b 1
)

:run
echo  [OK] tester ready. If the dashboard is open, close it first.
echo.
"%BIN%" %*

echo.
pause
