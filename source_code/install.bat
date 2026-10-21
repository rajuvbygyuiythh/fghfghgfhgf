@echo off
REM ============================================================================
REM  ChitChat Bot (EVAChat Flow + API Chat) - smart installer / builder
REM
REM  What it does:
REM    1. Installs Go (>= 1.24)           -> winget, or official MSI from go.dev
REM    2. Installs the Wails v2 CLI        -> go install ...wails/v2/cmd/wails@v2.12.0
REM    3. Installs Microsoft WebView2      -> winget, or the official bootstrapper
REM    4. Builds the dashboard             -> wails build -clean -skipbindings -m -s -webview2 error -o chitchatui.exe
REM       (fallback: go build -tags desktop,production)
REM    5. Builds the CLI worker & tester   -> go build .\cmd\chitchatbot & .\cmd\chitchattest
REM    6. Offers to start build\bin\chitchatui.exe (or starts immediately in 'run' mode)
REM
REM  Usage:   install.bat            full run (install + build + ask to run)
REM           install.bat build      skip installs, only build
REM           install.bat run        build if missing, then start the dashboard
REM           install.bat bundle     full portable bundle (WebView2 + Chromium)
REM                                  via scripts\bundle-windows.ps1 -> dist\windows-x64
REM  Log:     build\install.log
REM ============================================================================
setlocal EnableExtensions DisableDelayedExpansion
title ChitChat Bot - Install and Build
cd /d "%~dp0"
set "ROOT=%CD%"
set "BIN=%ROOT%\build\bin"
set "LOG=%ROOT%\build\install.log"
set "WAILS_VERSION=v2.12.0"
set "GO_MIN_MINOR=24"
set "MODE=%~1"
if "%MODE%"=="" set "MODE=all"

if not exist "%ROOT%\build" mkdir "%ROOT%\build"
echo [%date% %time%] install.bat mode=%MODE% >> "%LOG%"

echo.
echo  ==================================================
echo   ChitChat Bot  ^|  mode: %MODE%
echo   project: %ROOT%
echo  ==================================================
echo.

if not exist "%ROOT%\wails.json" (
    call :fail "wails.json not found. Run install.bat from the project root folder."
)

REM make sure Go / GOPATH bin are visible even right after a fresh install
set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"

if /i "%MODE%"=="run"    goto :run
if /i "%MODE%"=="build"  goto :build
if /i "%MODE%"=="bundle" goto :bundle
if /i not "%MODE%"=="all" call :fail "Unknown mode '%MODE%'. Use: install.bat  all / build / run / bundle"

REM ---------------------------------------------------------------- 1. Go ---
:step_go
call :log "Step 1/5  Go toolchain"
call :check_go
if "%GO_OK%"=="1" (
    echo   [OK] Go found: %GO_VER%
    goto :step_wails
)
echo   [..] Go 1.%GO_MIN_MINOR%+ not found - installing...
call :has_winget
if "%WINGET_OK%"=="1" (
    winget install --id GoLang.Go -e --silent --accept-package-agreements --accept-source-agreements >> "%LOG%" 2>&1
)
set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"
call :check_go
if "%GO_OK%"=="1" (
    echo   [OK] Go installed: %GO_VER%
    goto :step_wails
)
echo   [..] winget route failed - downloading the official Go MSI from go.dev ...
if exist "%ROOT%\build\go_msi_name.txt" del "%ROOT%\build\go_msi_name.txt"
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ProgressPreference='SilentlyContinue';" ^
  "$rel = (Invoke-RestMethod 'https://go.dev/dl/?mode=json') | Where-Object { $_.stable } | Select-Object -First 1;" ^
  "$f = $rel.files | Where-Object { $_.os -eq 'windows' -and $_.arch -eq 'amd64' -and $_.kind -eq 'installer' } | Select-Object -First 1;" ^
  "if (-not $f) { throw 'no windows-amd64 installer in the go.dev list' };" ^
  "$out = Join-Path $env:ROOT 'build' | Join-Path -ChildPath $f.filename;" ^
  "Write-Host ('  downloading ' + $f.filename);" ^
  "Invoke-WebRequest -UseBasicParsing ('https://go.dev/dl/' + $f.filename) -OutFile $out;" ^
  "$namePath = Join-Path (Join-Path $env:ROOT 'build') 'go_msi_name.txt';" ^
  "Set-Content -LiteralPath $namePath -Value $f.filename -NoNewline" >> "%LOG%" 2>&1
set "GO_MSI="
if exist "%ROOT%\build\go_msi_name.txt" set /p GO_MSI=<"%ROOT%\build\go_msi_name.txt"
if defined GO_MSI (
    echo   [..] running %GO_MSI% ^(UAC prompt may appear^)
    msiexec /i "%ROOT%\build\%GO_MSI%" /qn /norestart
)
set "PATH=%ProgramFiles%\Go\bin;%LOCALAPPDATA%\Programs\Go\bin;%USERPROFILE%\go\bin;%PATH%"
call :check_go
if "%GO_OK%"=="1" (
    echo   [OK] Go installed: %GO_VER%
    goto :step_wails
)
call :fail "Go could not be installed automatically. Install Go 1.24+ from https://go.dev/dl and run install.bat again."

REM ------------------------------------------------------------- 2. Wails ---
:step_wails
call :log "Step 2/5  Wails CLI %WAILS_VERSION%"
for /f "delims=" %%p in ('go env GOPATH 2^>nul') do set "GOPATH_DIR=%%p"
if defined GOPATH_DIR set "PATH=%GOPATH_DIR%\bin;%PATH%"
call :check_wails
if "%WAILS_OK%"=="1" (
    echo   [OK] Wails CLI found: %WAILS_VER%
    goto :step_webview2
)
echo   [..] Installing Wails CLI (go install, needs internet, 1-3 min) ...
go install github.com/wailsapp/wails/v2/cmd/wails@%WAILS_VERSION% >> "%LOG%" 2>&1
call :check_wails
if "%WAILS_OK%"=="1" (
    echo   [OK] Wails CLI installed: %WAILS_VER%
    goto :step_webview2
)
echo   [!!] Wails CLI install failed - the build step will use the plain 'go build' fallback.
goto :step_webview2

REM ---------------------------------------------------------- 3. WebView2 ---
:step_webview2
call :log "Step 3/5  Microsoft Edge WebView2 runtime"
call :check_webview2
if "%WV2_OK%"=="1" (
    echo   [OK] WebView2 runtime present.
    goto :step_node
)
echo   [..] WebView2 runtime missing - installing (UAC prompt may appear) ...
call :has_winget
if "%WINGET_OK%"=="1" (
    winget install --id Microsoft.EdgeWebView2Runtime -e --silent --accept-package-agreements --accept-source-agreements >> "%LOG%" 2>&1
)
call :check_webview2
if "%WV2_OK%"=="1" (
    echo   [OK] WebView2 runtime installed.
    goto :step_node
)
powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ProgressPreference='SilentlyContinue';" ^
  "$out = Join-Path (Join-Path $env:ROOT 'build') 'MicrosoftEdgeWebview2Setup.exe';" ^
  "Invoke-WebRequest -UseBasicParsing 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $out;" ^
  "Start-Process $out -ArgumentList '/silent','/install' -Wait" >> "%LOG%" 2>&1
call :check_webview2
if "%WV2_OK%"=="1" (
    echo   [OK] WebView2 runtime installed.
) else (
    echo   [!!] WebView2 could not be verified. Windows 11 / updated Windows 10 usually has it.
    echo        If the dashboard does not open, install it from:
    echo        https://developer.microsoft.com/microsoft-edge/webview2/
)

REM ------------------------------------------------------------- 4. Node ---
:step_node
call :log "Step 4/5  Node.js (optional)"
where npm >nul 2>&1
if errorlevel 1 (
    echo   [i ] npm not found - not needed: the dashboard frontend is pre-built
    echo        in cmd\chitchatui\frontend\dist and embedded into the exe.
) else (
    echo   [OK] npm found - fine, but not required.
)

REM ------------------------------------------------------------- 5. Build ---
:build
call :log "Step 5/5  Build"
call :check_go
if not "%GO_OK%"=="1" (
    if /i "%MODE%"=="build" (
        echo   [..] Go not found yet - running full setup first...
        goto :step_go
    )
    call :fail "Go 1.24+ is required to build. Run 'install.bat' (full mode) first."
)
for /f "delims=" %%p in ('go env GOPATH 2^>nul') do set "GOPATH_DIR=%%p"
if defined GOPATH_DIR set "PATH=%GOPATH_DIR%\bin;%PATH%"
if not exist "%BIN%" mkdir "%BIN%"

REM A running dashboard locks build\bin\chitchatui.exe; remove any old exe first.
if exist "%BIN%\chitchatui.exe" (
    del /f /q "%BIN%\chitchatui.exe" >nul 2>&1
    if exist "%BIN%\chitchatui.exe" (
        echo   [X] This project's dashboard is running and locks build\bin\chitchatui.exe.
        echo       Close the ChitChat Bot window, then run install.bat again.
        call :fail "Close the running dashboard before building."
    )
)

set "GOFLAGS="
set "GOWORK=off"
set "CGO_ENABLED=0"

echo   [..] Downloading Go dependencies ^(first run may take 1-2 min^)...
go mod download >> "%LOG%" 2>&1

call :check_wails
if "%WAILS_OK%"=="1" (
    echo   [..] wails build -clean -skipbindings -m -s -webview2 error -o chitchatui.exe
    wails build -clean -skipbindings -m -s -webview2 error -o chitchatui.exe >> "%LOG%" 2>&1
    if not errorlevel 1 if exist "%BIN%\chitchatui.exe" (
        echo   [OK] build\bin\chitchatui.exe built with Wails.
        goto :build_cli
    )
    echo   [!!] wails build failed - falling back to direct Go build...
)
echo   [..] go build -tags desktop,production ^(Wails-compatible desktop build^)
go build -tags desktop,production -ldflags "-w -s -H windowsgui" -o "%BIN%\chitchatui.exe" . >> "%LOG%" 2>&1
if not exist "%BIN%\chitchatui.exe" (
    echo.
    echo   --- Last lines from build\install.log ---
    powershell -NoProfile -ExecutionPolicy Bypass -Command "if (Test-Path -LiteralPath $env:LOG) { Get-Content -LiteralPath $env:LOG -Tail 25 }" 2>nul
    call :fail "Dashboard build failed. Open build\install.log for the compiler output."
)
echo   [OK] build\bin\chitchatui.exe built.

:build_cli
echo   [..] go build .\cmd\chitchatbot  ^(CLI worker^)
go build -o "%BIN%\chitchatbot.exe" .\cmd\chitchatbot >> "%LOG%" 2>&1
if exist "%BIN%\chitchatbot.exe" (
    echo   [OK] build\bin\chitchatbot.exe built.
) else (
    echo   [!!] CLI worker build failed ^(dashboard still usable^). See build\install.log.
)

echo   [..] go build .\cmd\chitchattest ^(local console tester^)
go build -o "%BIN%\chitchattest.exe" .\cmd\chitchattest >> "%LOG%" 2>&1
if exist "%BIN%\chitchattest.exe" (
    echo   [OK] build\bin\chitchattest.exe built.
)

echo   [..] go build .\cmd\apichat ^(API Persona Studio ^& Live Test Chat GUI^)
go build -tags desktop,production -ldflags "-w -s -H windowsgui" -o "%BIN%\apichat.exe" .\cmd\apichat >> "%LOG%" 2>&1
if exist "%BIN%\apichat.exe" (
    echo   [OK] build\bin\apichat.exe built.
)

echo.
echo  ==================================================
echo   DONE.  Dashboard exe : build\bin\chitchatui.exe
echo          API Studio    : build\bin\apichat.exe  ^(or run apichat.bat^)
echo          CLI worker    : build\bin\chitchatbot.exe
echo          Local tester  : build\bin\chitchattest.exe
echo          Runtime data  : %ROOT%\data\   ^(created on first run^)
echo          EVAChat TXT   : %ROOT%\data\eva_chat\  ^(seeded on first EVAChat start^)
echo   Next time just double-click run-dashboard.bat or apichat.bat
echo  ==================================================
echo.
if /i "%MODE%"=="build" goto :end
if /i "%MODE%"=="run"   goto :launch_exe
choice /C YN /N /M "  Start the dashboard now? [Y/N] "
if errorlevel 2 goto :end
goto :launch_exe

REM --------------------------------------------------------------- run ---
:run
if not exist "%BIN%\chitchatui.exe" (
    echo   [i ] build\bin\chitchatui.exe not found yet - building it now...
    goto :step_go
)
:launch_exe
if not exist "%BIN%\chitchatui.exe" call :fail "build\bin\chitchatui.exe not found. Run install.bat first."
echo   [..] Starting dashboard ^(data folder: %ROOT%\data^)
cd /d "%ROOT%"
start "" "%BIN%\chitchatui.exe"
goto :end

REM ------------------------------------------------------------ bundle ---
:bundle
call :check_go
if not "%GO_OK%"=="1" call :fail "Go 1.24+ is required. Run 'install.bat' (full mode) first."
call :check_wails
if not "%WAILS_OK%"=="1" call :fail "Wails CLI is required for the bundle. Run 'install.bat' (full mode) first."
echo   [..] Building the portable bundle (downloads WebView2 fixed runtime + Chromium, several hundred MB)...
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%\scripts\bundle-windows.ps1" -Clean
if errorlevel 1 call :fail "bundle-windows.ps1 failed."
echo   [OK] Bundle ready: %ROOT%\dist\windows-x64\
goto :end

REM ============================================================ helpers ===
:check_go
set "GO_OK=0"
set "GO_VER="
for /f "tokens=3" %%v in ('go version 2^>nul') do set "GO_VER=%%v"
if not defined GO_VER exit /b 0
set "GO_NUM=%GO_VER:go=%"
for /f "tokens=1,2 delims=.rcbeta" %%a in ("%GO_NUM%") do (
    set "GO_MAJOR=%%a"
    set "GO_MINOR=%%b"
)
if not defined GO_MINOR exit /b 0
if %GO_MAJOR% GTR 1 set "GO_OK=1"
if %GO_MAJOR% EQU 1 if %GO_MINOR% GEQ %GO_MIN_MINOR% set "GO_OK=1"
exit /b 0

:check_wails
set "WAILS_OK=0"
set "WAILS_VER="
for /f "tokens=*" %%v in ('wails version 2^>nul') do set "WAILS_VER=%%v"
if defined WAILS_VER set "WAILS_OK=1"
exit /b 0

:check_webview2
set "WV2_OK=0"
reg query "HKLM\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv >nul 2>&1 && set "WV2_OK=1"
reg query "HKLM\SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv >nul 2>&1 && set "WV2_OK=1"
reg query "HKCU\Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" /v pv >nul 2>&1 && set "WV2_OK=1"
exit /b 0

:has_winget
set "WINGET_OK=0"
where winget >nul 2>&1 && set "WINGET_OK=1"
if "%WINGET_OK%"=="0" echo   [i ] winget not available - using direct downloads.
exit /b 0

:log
echo.
echo  --- %~1 ---
echo [%date% %time%] %~1 >> "%LOG%"
exit /b 0

:fail
echo.
echo   [ERROR] %~1
echo [%date% %time%] ERROR %~1 >> "%LOG%"
echo.
pause
exit 1

:end
echo.
echo  Local chat test: run test.bat to try all 4 dashboard Message options
echo  (nothing is sent to the real app; close the dashboard first).
echo.
echo [%date% %time%] finished mode=%MODE% >> "%LOG%"
endlocal
exit /b 0
