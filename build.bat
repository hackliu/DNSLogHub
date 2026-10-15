@echo off
setlocal enabledelayedexpansion
REM DNSLogHUB cross-compile script (Windows, equivalent to build.sh)
REM Usage: build.bat [clean]

if /I "%~1"=="clean" (
  if exist dist rmdir /s /q dist
  echo cleaned dist/
  exit /b 0
)

set "VERSION=dev"
set "COMMIT=unknown"
for /f "delims=" %%i in ('git describe --tags --always --dirty 2^>nul') do set "VERSION=%%i"
for /f "delims=" %%i in ('git rev-parse --short HEAD 2^>nul') do set "COMMIT=%%i"

set "LDFLAGS=-s -w -X main.version=%VERSION% -X main.commit=%COMMIT% -X main.buildTime=unknown"

if exist dist rmdir /s /q dist
mkdir dist

call :build linux amd64
call :build linux arm64
call :build windows amd64
call :build windows arm64
call :build darwin amd64
call :build darwin arm64

echo.
echo build done:
dir /b dist
exit /b 0

:build
set "GOOS=%~1"
set "GOARCH=%~2"
set "EXE="
if "%GOOS%"=="windows" set "EXE=.exe"
set "SERVER=dnsloghub-%GOOS%-%GOARCH%%EXE%"
set "INV=invitegen-%GOOS%-%GOARCH%%EXE%"

echo building %GOOS%/%GOARCH%
set "CGO_ENABLED=0"
go build -trimpath -ldflags "%LDFLAGS%" -o "dist\%SERVER%" ./cmd/server || exit /b 1
go build -trimpath -ldflags "%LDFLAGS%" -o "dist\%INV%" ./cmd/invitegen || exit /b 1

set "STAGE=dist\.stage-%GOOS%-%GOARCH%"
if exist "%STAGE%" rmdir /s /q "%STAGE%"
mkdir "%STAGE%"
copy "dist\%SERVER%" "%STAGE%\" >nul
copy "dist\%INV%" "%STAGE%\" >nul
copy config.yaml "%STAGE%\" >nul
if exist scripts xcopy /e /i /y scripts "%STAGE%\scripts" >nul

set "ARCHIVE=dnsloghub-%GOOS%-%GOARCH%.tar.gz"
tar -czf "dist\%ARCHIVE%" -C "%STAGE%" .
rmdir /s /q "%STAGE%"

certutil -hashfile "dist\%ARCHIVE%" SHA256 | findstr /v "hash CertUtil" > "dist\%ARCHIVE%.sha256"

exit /b 0
