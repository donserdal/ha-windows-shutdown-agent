#!/usr/bin/env python3
"""
Home Assistant – Windows Shutdown Client
=========================================
Een lichtgewicht HTTP-API-server die Home Assistant toestaat
deze Windows-computer te besturen.  Geen beheerdersrechten nodig.

Gebruik:
    client.py [opties]

Opties:
    --port PORT           Luisterpoort (standaard: 8765)
    --delay SECONDEN      Standaard vertraging in seconden (standaard: 30)
    --type TYPE           Standaard afsluittype (shutdown/restart/hibernate/sleep/logoff)
    --save-defaults       Sla --port/--delay/--type op in het register en sluit af
    --show-password       Toon het huidige API-wachtwoord en sluit af
    --reset-password      Genereer een nieuw API-wachtwoord
    --no-tray             Draai zonder systeemvak-pictogram (consolemodus)
"""

from __future__ import annotations

import argparse
import asyncio
import json
import logging
import os
import secrets
import socket
import subprocess
import sys
import threading
import time
import winreg
from functools import wraps
from typing import Any

# ---------------------------------------------------------------------------
# Derde-partij-imports (zie requirements.txt)
# ---------------------------------------------------------------------------
from cryptography.fernet import Fernet, InvalidToken
from flask import Flask, abort, jsonify, request
from zeroconf import IPVersion, ServiceInfo, Zeroconf

try:
    import pystray
    from PIL import Image, ImageDraw

    HAS_TRAY = True
except ImportError:
    HAS_TRAY = False

# ---------------------------------------------------------------------------
# Constanten
# ---------------------------------------------------------------------------
VERSION = "1.0.0"
SERVICE_TYPE = "_ha-shutdown._tcp.local."
DEFAULT_PORT = 8765
DEFAULT_DELAY = 30          # seconden
DEFAULT_TYPE = "shutdown"

# Register-pad  (HKEY_CURRENT_USER\Software\HAShutdownClient)
REG_ROOT = winreg.HKEY_CURRENT_USER
REG_PATH = r"Software\HAShutdownClient"

SHUTDOWN_TYPES: dict[str, str] = {
    "shutdown":  "Afsluiten",
    "restart":   "Opnieuw opstarten",
    "hibernate": "Slaapstand",
    "sleep":     "Sluimerstand",
    "logoff":    "Afmelden",
}

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    handlers=[logging.StreamHandler(sys.stdout)],
)
log = logging.getLogger("ha-shutdown")

# Onderdruk Flask-startup-banner
cli = sys.modules.get("flask.cli")
if cli:
    cli.show_server_banner = lambda *_: None  # type: ignore[attr-defined]

app = Flask(__name__)
app.logger.setLevel(logging.WARNING)

# ---------------------------------------------------------------------------
# Register-hulpfuncties
# ---------------------------------------------------------------------------

def _open_reg(write: bool = False):
    """Open of maak de register-sleutel aan (HKCU, geen beheerdersrechten)."""
    access = winreg.KEY_READ | (winreg.KEY_SET_VALUE if write else 0)
    try:
        return winreg.OpenKey(REG_ROOT, REG_PATH, 0, access)
    except FileNotFoundError:
        return winreg.CreateKeyEx(
            REG_ROOT, REG_PATH, 0, winreg.KEY_READ | winreg.KEY_SET_VALUE
        )


def reg_get(name: str, default: str | None = None) -> str | None:
    """Lees een REG_SZ-waarde; geef *default* terug als de waarde niet bestaat."""
    try:
        with _open_reg() as k:
            val, _ = winreg.QueryValueEx(k, name)
            return str(val)
    except (FileNotFoundError, OSError):
        return default


def reg_set(name: str, value: str) -> None:
    """Schrijf een REG_SZ-waarde."""
    with _open_reg(write=True) as k:
        winreg.SetValueEx(k, name, 0, winreg.REG_SZ, value)


def reg_delete(name: str) -> None:
    """Verwijder een register-waarde (negeert fouten)."""
    try:
        with _open_reg(write=True) as k:
            winreg.DeleteValue(k, name)
    except (FileNotFoundError, OSError):
        pass


# ---------------------------------------------------------------------------
# Fernet-versleuteling
# ---------------------------------------------------------------------------

def _get_fernet() -> Fernet:
    """Haal de Fernet-sleutel op uit het register; genereer hem indien nodig."""
    raw = reg_get("FernetKey")
    if not raw:
        raw = Fernet.generate_key().decode()
        reg_set("FernetKey", raw)
    return Fernet(raw.encode())


def get_api_password(generate: bool = True) -> str | None:
    """
    Lees het versleuteld API-wachtwoord uit het register.
    Als het niet bestaat en *generate* is True, wordt er een nieuw wachtwoord
    aangemaakt en opgeslagen.
    """
    f = _get_fernet()
    enc = reg_get("EncryptedPassword")
    if enc:
        try:
            return f.decrypt(enc.encode()).decode()
        except (InvalidToken, Exception):
            # Versleuteling ongeldig → verwijder en genereer opnieuw
            reg_delete("EncryptedPassword")

    if not generate:
        return None

    password = secrets.token_urlsafe(32)
    reg_set("EncryptedPassword", f.encrypt(password.encode()).decode())
    log.info("Nieuw API-wachtwoord gegenereerd: %s", password)
    return password


def reset_api_password() -> str:
    """Genereer een nieuw API-wachtwoord, sla het op en geef het terug."""
    f = _get_fernet()
    password = secrets.token_urlsafe(32)
    reg_set("EncryptedPassword", f.encrypt(password.encode()).decode())
    log.info("API-wachtwoord opnieuw ingesteld: %s", password)
    return password


# ---------------------------------------------------------------------------
# Instellingen
# ---------------------------------------------------------------------------

class Settings:
    """Laadt/slaat instellingen op via het Windows-register (HKCU)."""

    def __init__(self) -> None:
        self.port: int = int(reg_get("Port") or DEFAULT_PORT)
        self.delay: int = int(reg_get("ShutdownDelay") or DEFAULT_DELAY)
        self.shutdown_type: str = reg_get("ShutdownType") or DEFAULT_TYPE

    def save_to_registry(self) -> None:
        reg_set("Port", str(self.port))
        reg_set("ShutdownDelay", str(self.delay))
        reg_set("ShutdownType", self.shutdown_type)
        log.info(
            "Instellingen opgeslagen → poort=%d, vertraging=%ds, type=%s",
            self.port, self.delay, self.shutdown_type,
        )


settings = Settings()


# ---------------------------------------------------------------------------
# Authenticatie-decorator
# ---------------------------------------------------------------------------

def require_auth(func):
    """Controleer de X-API-Key-header; geeft 401 bij een ongeldige sleutel."""
    @wraps(func)
    def wrapper(*args, **kwargs):
        token = (
            request.headers.get("X-API-Key", "")
            or request.args.get("api_key", "")
        )
        expected = get_api_password(generate=False)
        if not expected or not token or token != expected:
            abort(401)
        return func(*args, **kwargs)
    return wrapper


# ---------------------------------------------------------------------------
# Afsluitlogica
# ---------------------------------------------------------------------------

def _build_cmd(stype: str, delay: int) -> list[str]:
    commands = {
        "shutdown":  ["shutdown", "/s", "/t", str(delay), "/f"],
        "restart":   ["shutdown", "/r", "/t", str(delay), "/f"],
        "hibernate": ["shutdown", "/h"],
        "sleep":     ["rundll32.exe", "powrprof.dll,SetSuspendState", "0,1,0"],
        "logoff":    ["shutdown", "/l"],
    }
    return commands.get(stype, commands["shutdown"])


def execute_shutdown(stype: str, delay: int) -> None:
    cmd = _build_cmd(stype, delay)
    log.info("Voert uit: %s", " ".join(cmd))
    subprocess.Popen(
        cmd,
        creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        close_fds=True,
    )


# ---------------------------------------------------------------------------
# Flask-eindpunten
# ---------------------------------------------------------------------------

@app.route("/status", methods=["GET"])
def status():
    """Publiek eindpunt – geen authenticatie vereist."""
    return jsonify({
        "status":   "online",
        "hostname": socket.gethostname(),
        "version":  VERSION,
    })


@app.route("/verify", methods=["GET"])
@require_auth
def verify():
    """Authenticatietest – vereist geldige API-sleutel, voert geen actie uit."""
    return jsonify({"status": "authenticated", "hostname": socket.gethostname()})


@app.route("/shutdown", methods=["POST"])
@require_auth
def shutdown():
    """
    Beveiligd eindpunt – start het afsluitproces.

    Optionele JSON-body:
        {
            "delay": 30,        // seconden (overschrijft standaardinstelling)
            "type": "shutdown"  // shutdown|restart|hibernate|sleep|logoff
        }
    """
    data: dict[str, Any] = request.get_json(silent=True) or {}
    delay = int(data.get("delay", settings.delay))
    stype = data.get("type", settings.shutdown_type)

    if stype not in SHUTDOWN_TYPES:
        return jsonify({"error": f"Ongeldig type. Gebruik: {list(SHUTDOWN_TYPES)}"}), 400
    if delay < 0:
        return jsonify({"error": "Vertraging moet ≥ 0 zijn"}), 400

    threading.Thread(
        target=execute_shutdown, args=(stype, delay), daemon=True
    ).start()

    return jsonify({
        "status":        "gestart",
        "type":          stype,
        "delay_seconds": delay,
    })


# ---------------------------------------------------------------------------
# mDNS-advertentie (Zeroconf)
# ---------------------------------------------------------------------------

def start_mdns(port: int) -> Zeroconf:
    """Adverteer de service via mDNS zodat Home Assistant hem kan vinden."""
    hostname = socket.gethostname()
    try:
        ip = socket.gethostbyname(hostname)
    except socket.gaierror:
        # Fallback: gebruik het eerste niet-loopback-adres
        ip = "127.0.0.1"
        for line in subprocess.check_output(["ipconfig"], text=True).splitlines():
            if "IPv4" in line and "127.0.0.1" not in line:
                parts = line.split(":")
                if len(parts) == 2:
                    ip = parts[1].strip()
                    break

    info = ServiceInfo(
        SERVICE_TYPE,
        f"{hostname}.{SERVICE_TYPE}",
        addresses=[socket.inet_aton(ip)],
        port=port,
        properties={
            b"hostname": hostname.encode(),
            b"version":  VERSION.encode(),
        },
    )
    zc = Zeroconf(ip_version=IPVersion.V4Only)
    zc.register_service(info)
    log.info("mDNS: %s geadverteerd op %s:%d", hostname, ip, port)
    return zc


# ---------------------------------------------------------------------------
# Systeemvak-pictogram
# ---------------------------------------------------------------------------

def _make_tray_image() -> "Image.Image":
    """Maak een eenvoudig aan/uit-knop-pictogram."""
    size = 64
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    # Blauwe cirkel
    d.ellipse([2, 2, 62, 62], fill=(41, 128, 185, 255))
    # Witte aan/uit-streep
    d.rectangle([30, 10, 34, 30], fill=(255, 255, 255, 255))
    # Witte boog (aan/uit-symbool)
    d.arc([14, 16, 50, 52], start=40, end=140, fill=(255, 255, 255, 255), width=4)
    return img


def run_tray(password: str) -> None:
    """Start het systeemvak-pictogram (blokkeert totdat de gebruiker Afsluiten kiest)."""
    if not HAS_TRAY:
        log.warning("pystray/Pillow niet beschikbaar. Draait zonder systeemvak.")
        try:
            while True:
                time.sleep(3600)
        except KeyboardInterrupt:
            pass
        return

    def show_info(icon, item):  # noqa: ARG001
        import tkinter as tk
        from tkinter import messagebox
        root = tk.Tk()
        root.withdraw()
        root.attributes("-topmost", True)
        messagebox.showinfo(
            "HA Shutdown Client",
            f"Status:    Actief ✓\n"
            f"Hostnaam:  {socket.gethostname()}\n"
            f"Poort:     {settings.port}\n\n"
            f"API-sleutel:\n{password}",
        )
        root.destroy()

    def quit_handler(icon, item):  # noqa: ARG001
        icon.stop()

    menu = pystray.Menu(
        pystray.MenuItem("ℹ  Verbindingsinfo tonen", show_info, default=True),
        pystray.Menu.SEPARATOR,
        pystray.MenuItem("✕  Afsluiten", quit_handler),
    )
    icon = pystray.Icon(
        "HA Shutdown Client",
        _make_tray_image(),
        "HA Shutdown Client",
        menu=menu,
    )
    icon.run()  # blokkeert


# ---------------------------------------------------------------------------
# CLI-argumenten
# ---------------------------------------------------------------------------

def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="ha-shutdown-client",
        description="Home Assistant Windows Shutdown Client",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=f"""
Afsluittypen:
  shutdown   Afsluiten (standaard)
  restart    Opnieuw opstarten
  hibernate  Slaapstand (vereist hiberfil.sys)
  sleep      Sluimerstand
  logoff     Huidige gebruiker afmelden

Voorbeelden:
  %(prog)s
  %(prog)s --delay 60 --type restart --save-defaults
  %(prog)s --show-password
  %(prog)s --reset-password
  %(prog)s --no-tray --port 9000
""",
    )
    p.add_argument(
        "--port", type=int, metavar="POORT",
        help=f"Luisterpoort (standaard: {DEFAULT_PORT})",
    )
    p.add_argument(
        "--delay", type=int, metavar="SECONDEN",
        help=f"Standaard vertraging vóór afsluiten (standaard: {DEFAULT_DELAY}s)",
    )
    p.add_argument(
        "--type", dest="shutdown_type",
        choices=list(SHUTDOWN_TYPES.keys()), metavar="TYPE",
        help="Standaard afsluittype: " + ", ".join(SHUTDOWN_TYPES.keys()),
    )
    p.add_argument(
        "--save-defaults", action="store_true",
        help="Sla --port/--delay/--type op in het register en sluit het programma af",
    )
    p.add_argument(
        "--show-password", action="store_true",
        help="Toon het huidige API-wachtwoord en sluit het programma af",
    )
    p.add_argument(
        "--reset-password", action="store_true",
        help="Genereer een nieuw API-wachtwoord en sla dit op",
    )
    p.add_argument(
        "--no-tray", action="store_true",
        help="Draai zonder systeemvak-pictogram (consolemodus)",
    )
    return p


# ---------------------------------------------------------------------------
# Hoofdprogramma
# ---------------------------------------------------------------------------

def main() -> None:
    parser = build_parser()
    args = parser.parse_args()

    # ── Pas CLI-overschrijvingen toe ─────────────────────────────────────────
    if args.port is not None:
        settings.port = args.port
    if args.delay is not None:
        settings.delay = args.delay
    if args.shutdown_type is not None:
        settings.shutdown_type = args.shutdown_type

    # ── Sub-opdrachten die meteen afsluiten ──────────────────────────────────
    if args.save_defaults:
        settings.save_to_registry()
        print("✓ Standaardinstellingen opgeslagen in het Windows-register.")
        return

    if args.reset_password:
        pw = reset_api_password()
        print(f"✓ Nieuw API-wachtwoord: {pw}")
        print("Voer dit wachtwoord in bij het instellen van Home Assistant.")
        return

    # Wachtwoord ophalen (of genereren bij eerste start)
    password = get_api_password()

    if args.show_password:
        print(f"API-wachtwoord : {password}")
        print(f"Poort          : {settings.port}")
        print(f"Vertraging     : {settings.delay}s")
        print(f"Afsluittype    : {settings.shutdown_type}")
        return

    # ── Start de service ─────────────────────────────────────────────────────
    separator = "=" * 52
    log.info(separator)
    log.info("  HA Windows Shutdown Client  v%s", VERSION)
    log.info(separator)
    log.info("  Hostnaam    : %s", socket.gethostname())
    log.info("  Poort       : %d", settings.port)
    log.info("  API-sleutel : %s", password)
    log.info("  Type        : %s", settings.shutdown_type)
    log.info("  Vertraging  : %ds", settings.delay)
    log.info(separator)
    log.info("Voer de API-sleutel in bij het instellen van Home Assistant.")
    log.info(separator)

    # Start mDNS-advertentie
    zc = start_mdns(settings.port)

    # Start Flask-API in achtergrond-thread
    flask_thread = threading.Thread(
        target=lambda: app.run(
            host="0.0.0.0",
            port=settings.port,
            debug=False,
            use_reloader=False,
        ),
        daemon=True,
        name="flask-api",
    )
    flask_thread.start()
    log.info("API-server gestart op poort %d", settings.port)

    try:
        if args.no_tray:
            log.info("Druk op Ctrl+C om te stoppen.")
            flask_thread.join()
        else:
            run_tray(password)          # blokkeert tot de gebruiker kiest voor Afsluiten
    except KeyboardInterrupt:
        pass
    finally:
        log.info("Client wordt gestopt...")
        zc.unregister_all_services()
        zc.close()


if __name__ == "__main__":
    main()
