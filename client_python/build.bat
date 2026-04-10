@echo off
:: ============================================================
::  HA Windows Shutdown Client – bouw een zelfstandig .exe
:: ============================================================
:: Vereisten: Python 3.11+, pip install pyinstaller
::
:: Uitvoer: dist\ha-shutdown-client.exe
:: ============================================================

echo [1/3] Controleer Python...
python --version || (echo Python niet gevonden. && pause && exit /b 1)

echo [2/3] Installeer afhankelijkheden...
pip install -r requirements.txt

echo [3/3] Bouw executable...
pyinstaller ^
    --onefile ^
    --noconsole ^
    --name "ha-shutdown-client" ^
    --icon NONE ^
    --add-data "." ^
    client.py

echo.
echo ============================================================
echo  Klaar!  Uitvoerbestand: dist\ha-shutdown-client.exe
echo ============================================================
pause
