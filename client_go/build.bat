@echo off
:: ============================================================
::  HA Windows Shutdown Client (Go) – bouwscript
:: ============================================================
::
::  Gebruik:
::    build.bat          Bouw met consolevenster (voor debuggen)
::    build.bat release  Bouw zonder consolevenster (voor productie)
::
::  Vereisten: Go 1.21+  (https://go.dev/dl/)
:: ============================================================

setlocal

set APPNAME=ha-shutdown-client
set GOOS=windows
set GOARCH=amd64

echo.
echo  HA Windows Shutdown Client – Go-bouw
echo  =====================================

:: Controleer of Go beschikbaar is
go version >nul 2>&1
if errorlevel 1 (
    echo  [FOUT] Go is niet gevonden. Installeer Go via https://go.dev/dl/
    pause & exit /b 1
)

echo  [1/5] Go-versie:
go version

echo.
echo  [2/5] Afhankelijkheden ophalen ^(go mod tidy^)...
go mod tidy
if errorlevel 1 (
    echo  [FOUT] 'go mod tidy' mislukt.
    pause & exit /b 1
)

:: ── Manifest + icoon inbakken via rsrc ──────────────────────────────────────
:: rsrc genereert rsrc.syso: een Windows-resource-bestand dat de compiler
:: automatisch meeneemt. Het bevat:
::   - app.manifest  → comctl32 v6 (moderne controls) + DPI-bewustzijn
::   - .ico          → applicatie-icoon zichtbaar in Verkenner en taakbalk
echo.
echo  [3/5] Manifest + icoon inbakken ^(rsrc^)...
rsrc -manifest app.manifest -ico homeassistant_windows_shutdown.ico -o rsrc.syso >nul 2>&1
if errorlevel 1 (
    echo  [INFO] rsrc niet gevonden. Installeren...
    go install github.com/akavel/rsrc@latest
    if errorlevel 1 (
        echo  [WAARSCHUWING] rsrc kon niet worden geinstalleerd.
        echo  De app werkt, maar het .exe-icoon en de moderne UI-stijl ontbreken.
        echo  Installeer handmatig: go install github.com/akavel/rsrc@latest
    ) else (
        rsrc -manifest app.manifest -ico homeassistant_windows_shutdown.ico -o rsrc.syso
    )
)

echo.
echo  [4/5] Code controleren ^(go vet^)...
:: -unsafeptr=false: clipboard.go gebruikt uintptr->unsafe.Pointer voor Windows-heap
:: geheugen (GlobalAlloc). De Go GC beheert dit niet en verplaatst het nooit,
:: waardoor de conversie altijd geldig is. go vet kan dit statisch niet onderscheiden
:: en geeft een false positive. Alle overige vet-checks blijven actief.
go vet -unsafeptr=false ./...
if errorlevel 1 (
    echo  [WAARSCHUWING] go vet meldde problemen. Controleer de uitvoer hierboven.
)

echo.
if "%1"=="release" (
    echo  [5/5] Bouwen in RELEASE-modus ^(geen consolevenster^)...
    go build -ldflags="-s -w -H windowsgui" -o "%APPNAME%.exe" .
) else (
    echo  [5/5] Bouwen in DEBUG-modus ^(met consolevenster^)...
    go build -ldflags="-s -w" -o "%APPNAME%.exe" .
)

if errorlevel 1 (
    echo  [FOUT] Bouwen mislukt.
    pause & exit /b 1
)

echo.
echo  ============================================================
echo   Klaar^^!  Uitvoerbestand: %APPNAME%.exe
if "%1"=="release" (
    echo   Modus: release ^(geen consolevenster, icoon ingebakken^)
) else (
    echo   Modus: debug ^(met consolevenster^)
    echo   Voor productie: build.bat release
)
echo  ============================================================
echo.
pause
