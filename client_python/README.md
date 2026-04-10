# HA Windows Shutdown Client

Een lichtgewicht HTTP-API-server die Home Assistant toestaat om deze Windows-computer
af te sluiten, opnieuw op te starten, in slaapstand te zetten, enzovoort.

**Geen beheerdersrechten nodig.** Alle instellingen worden opgeslagen in
`HKEY_CURRENT_USER\Software\HAShutdownClient`.

---

## Vereisten

- Windows 10 / 11
- Python 3.11 of nieuwer (of gebruik de voorgebouwde `.exe`)

---

## Installatie (Python)

```bat
pip install -r requirements.txt
python client.py
```

## Installatie (.exe)

```bat
build.bat          :: bouwt dist\ha-shutdown-client.exe
```

Kopieer `ha-shutdown-client.exe` naar een map naar keuze en voer het uit.

---

## Eerste start

Bij de eerste start wordt automatisch een willekeurig API-wachtwoord gegenereerd
en versleuteld (via Fernet-AES-256) opgeslagen in het Windows-register.

Het wachtwoord wordt weergegeven in:
- De console-uitvoer
- Het systeemvak-menu (klik op het pictogram → "Verbindingsinfo tonen")

**Noteer dit wachtwoord** – u hebt het nodig bij het instellen van de
Home Assistant-integratie.

---

## Opdrachtregelopties

| Optie | Beschrijving |
|-------|-------------|
| `--port POORT` | Luisterpoort (standaard: **8765**) |
| `--delay SECONDEN` | Standaard vertraging vóór afsluiten (standaard: **30s**) |
| `--type TYPE` | Standaard afsluittype (zie hieronder) |
| `--save-defaults` | Sla bovenstaande opties op in het register en sluit af |
| `--show-password` | Toon het huidige API-wachtwoord en sluit af |
| `--reset-password` | Genereer een nieuw API-wachtwoord |
| `--no-tray` | Draai zonder systeemvak (consolemodus) |

### Afsluittypen

| Type | Omschrijving |
|------|-------------|
| `shutdown` | Afsluiten *(standaard)* |
| `restart` | Opnieuw opstarten |
| `hibernate` | Slaapstand (hiberfil.sys vereist) |
| `sleep` | Sluimerstand |
| `logoff` | Huidige gebruiker afmelden |

### Voorbeelden

```bat
:: Sla een vertraging van 60s en type restart op als standaard
python client.py --delay 60 --type restart --save-defaults

:: Toon het API-wachtwoord
python client.py --show-password

:: Genereer een nieuw wachtwoord
python client.py --reset-password

:: Draai op poort 9000, zonder systeemvak
python client.py --port 9000 --no-tray
```

---

## API-eindpunten

### `GET /status`  *(geen authenticatie)*

Controleert of de client actief is.

```json
{ "status": "online", "hostname": "MIJN-PC", "version": "1.0.0" }
```

### `GET /verify`  *(X-API-Key vereist)*

Controleert de API-sleutel zonder actie uit te voeren.

```json
{ "status": "authenticated", "hostname": "MIJN-PC" }
```

### `POST /shutdown`  *(X-API-Key vereist)*

Start het afsluiten.  Optionele JSON-body:

```json
{ "delay": 30, "type": "shutdown" }
```

---

## Automatisch starten met Windows

1. Druk op `Win + R`, typ `shell:startup` en druk op Enter.
2. Maak een snelkoppeling naar `ha-shutdown-client.exe` (of `pythonw client.py`).
3. De client start nu automatisch mee bij Windows.

---

## Register-structuur (`HKCU\Software\HAShutdownClient`)

| Waarde | Type | Omschrijving |
|--------|------|-------------|
| `FernetKey` | REG_SZ | Fernet-encryptie-sleutel (automatisch aangemaakt) |
| `EncryptedPassword` | REG_SZ | Versleuteld API-wachtwoord |
| `Port` | REG_SZ | Luisterpoort |
| `ShutdownDelay` | REG_SZ | Standaard vertraging in seconden |
| `ShutdownType` | REG_SZ | Standaard afsluittype |
