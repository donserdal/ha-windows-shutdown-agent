# HA Windows Shutdown Client (Go)

Een lichtgewicht HTTP-API-server geschreven in **Go** waarmee Home Assistant
deze Windows-computer kan afsturen, opnieuw opstarten, in slaapstand zetten,
enzovoort.

**Geen beheerdersrechten nodig.**  
Alle instellingen staan in `HKEY_CURRENT_USER\Software\HAShutdownClient`.

---

## Kenmerken

| | |
|---|---|
| **Formaat** | Één zelfstandige `.exe`, geen installatie |
| **Grootte** | ≈ 8 MB (statisch gelinkt) |
| **Opstarten** | < 100 ms |
| **Beveiliging** | AES-256-GCM versleuteld wachtwoord in HKCU |
| **Ontdekking** | Automatisch via mDNS (Zeroconf) |
| **Systeemvak** | Pictogram met info, instellingen en taalkeuze |
| **Talen** | Nederlands en Engels (direct wisselbaar) |

---

## Vereisten

- Windows 10 / 11 (64-bit)
- [Go 1.21+](https://go.dev/dl/) *(alleen nodig voor bouwen)*

---

## Bouwen

```bat
:: Debug-build — met consolevenster, handig voor testen
build.bat

:: Release-build — geen consolevenster, voor dagelijks gebruik
build.bat release
```

Het resultaat is `ha-shutdown-client.exe` in de huidige map.

---

## Eerste start

Bij de eerste start worden automatisch:
1. Een AES-256-sleutel aangemaakt en opgeslagen in het register
2. Een willekeurig API-wachtwoord gegenereerd en **versleuteld** opgeslagen
3. Het systeemvak-pictogram getoond (blauw aan/uit-symbool)
4. Het wachtwoord afgedrukt in de console (indien beschikbaar)

Klik op het systeemvak-pictogram → **Verbindingsinfo tonen** om het
wachtwoord op te halen. Dit wachtwoord heeft u nodig bij het instellen
van de Home Assistant-integratie.

---

## Systeemvak-menu

```
ℹ  Verbindingsinfo tonen
⚙  Instellingen...
───────────────────
🌐  Taal
    ✓  English
       Nederlands
───────────────────
✕  Afsluiten
```

- **Verbindingsinfo** — toont hostnaam, poort en API-sleutel in een dialoogvenster
- **Instellingen** — opent een configuratievenster (zie hieronder)
- **Taal** — wisselt direct tussen Engels en Nederlands; alle labels worden meteen bijgewerkt
- **Afsluiten** — stopt de client (sluit de computer *niet* af)

---

## Instellingenvenster

Via **⚙ Instellingen...** opent een native Windows-dialoog:

```
┌─ HA Shutdown Client – Instellingen (MIJN-PC) ─────┐
│                                                     │
│  Luisterpoort:          [ 8765            ]         │
│  Vertraging (seconden): [ 30              ]         │
│  Afsluittype:           [ Afsluiten      ▼]         │
│  ─────────────────────────────────────────          │
│  API-sleutel:     [ abc123...  ] [ Vernieuwen ]      │
│  ─────────────────────────────────────────          │
│                          [ Opslaan ] [ Annuleren ]   │
└─────────────────────────────────────────────────────┘
```

Alle wijzigingen worden direct in het register opgeslagen.
Voor een nieuwe **poort** is een herstart van de client nodig.
**Vernieuwen** genereert een nieuw versleuteld API-wachtwoord.

---

## Opdrachtregelopties

```
ha-shutdown-client.exe [opties]

OPTIES
  --port POORT        Luisterpoort (standaard: 8765)
  --delay SECONDEN    Standaard vertraging voor afsluiten (standaard: 30)
  --type TYPE         Standaard afsluittype (zie hieronder)
  --save-defaults     Sla --port/--delay/--type op in het register
  --show-password     Toon het API-wachtwoord en sluit af
  --reset-password    Genereer een nieuw API-wachtwoord
  --no-tray           Consolemodus (geen systeemvak)
  --help              Toon het helpscherm
```

### Afsluittypen

| `--type` | Omschrijving |
|----------|-------------|
| `shutdown` | Afsluiten *(standaard)* |
| `restart` | Opnieuw opstarten |
| `hibernate` | Slaapstand *(vereist hiberfil.sys)* |
| `sleep` | Sluimerstand |
| `logoff` | Huidige gebruiker afmelden |

### Voorbeelden

```bat
:: Toon het API-wachtwoord (werkt ook vanuit cmd.exe / PowerShell)
ha-shutdown-client.exe --show-password

:: Sla een vertraging van 60s en type restart op als standaard
ha-shutdown-client.exe --delay 60 --type restart --save-defaults

:: Genereer een nieuw wachtwoord
ha-shutdown-client.exe --reset-password

:: Consolemodus op poort 9000
ha-shutdown-client.exe --port 9000 --no-tray
```

> **Console-overlap** — bij aanroep vanuit cmd.exe / PowerShell schrijft de
> client automatisch `\r\n` na het koppelen aan de parent-console, zodat
> uitvoer nooit overlapt met de al-afgedrukte prompt.

---

## API-eindpunten

De API-responses zijn altijd Engelstalig, ongeacht de UI-taalinstelling,
zodat Home Assistant ze altijd correct kan verwerken.

### `GET /status`  *(geen authenticatie)*

```json
{ "status": "online", "hostname": "MIJN-PC", "version": "1.0.0" }
```

### `GET /verify`  *(X-API-Key vereist)*

```http
X-API-Key: <sleutel>
```
```json
{ "status": "authenticated", "hostname": "MIJN-PC" }
```

### `POST /shutdown`  *(X-API-Key vereist)*

```http
X-API-Key: <sleutel>
Content-Type: application/json

{ "delay": 30, "type": "shutdown" }
```
```json
{ "status": "accepted", "type": "shutdown", "delay_seconds": 30 }
```

| HTTP-code | Betekenis |
|-----------|----------|
| `200` | Opdracht geaccepteerd |
| `400` | Ongeldig type of vertraging |
| `401` | Ongeldige API-sleutel |
| `405` | Geen POST-verzoek |

---

## Automatisch opstarten met Windows

1. Druk op `Win + R`, typ `shell:startup`, druk op Enter
2. Maak een snelkoppeling naar `ha-shutdown-client.exe` in die map
3. De client start nu automatisch bij het aanmelden

---

## Registerstructuur

Pad: `HKEY_CURRENT_USER\Software\HAShutdownClient`

| Waarde | Omschrijving |
|--------|-------------|
| `AESKey` | AES-256-sleutel (base64, automatisch aangemaakt) |
| `EncryptedPassword` | Versleuteld API-wachtwoord (AES-256-GCM, base64) |
| `Port` | Luisterpoort |
| `ShutdownDelay` | Standaard vertraging in seconden |
| `ShutdownType` | Standaard afsluittype |
| `Language` | UI-taal (`en` of `nl`, standaard `en`) |

---

## Home Assistant-integratie

Zie de map `custom_components/windows_shutdown/` voor de HA-integratie.

Na het installeren van de client:
1. Ga in HA naar **Instellingen → Integraties → Toevoegen**
2. Zoek op **Windows Shutdown**
3. Kies **Automatisch zoeken** of **Handmatig invoeren**
4. Voer de API-sleutel in (te vinden via systeemvak of `--show-password`)

De integratie voegt toe:
- **Binary sensor** `binary_sensor.<naam>_online` — aan/uit, nooit "niet beschikbaar"
- **Knop** `button.<naam>_shut_down` — stuurt de afsluit-opdracht
