//go:build windows

package main

import "sync/atomic"

// ──────────────────────────────────────────────────────────────────────────────
//  Taal-constanten
// ──────────────────────────────────────────────────────────────────────────────

const (
	LangEN = "en"
	LangNL = "nl"
)

// currentLang slaat de actieve taalcode op (atomisch, safe voor goroutines).
// Standaard: Engels.
var currentLang atomic.Value

func init() { currentLang.Store(LangEN) }

// SetLang stelt de actieve taal in en slaat die op in het register.
func SetLang(lang string) {
	if lang != LangEN && lang != LangNL {
		lang = LangEN
	}
	currentLang.Store(lang)
	_ = regSetString("Language", lang)
}

// GetLang geeft de huidige taalcode terug ("en" of "nl").
func GetLang() string {
	if v, ok := currentLang.Load().(string); ok {
		return v
	}
	return LangEN
}

// LoadLang laadt de taalvoorkeur uit het register.
// Moet worden aangeroepen bij het opstarten, na loadConfig().
func LoadLang() {
	lang := regGetString("Language", LangEN)
	currentLang.Store(lang)
}

// T geeft de vertaling van de gegeven sleutel terug in de actieve taal.
// Als de sleutel niet bestaat, wordt de sleutel zelf teruggegeven.
func T(key string) string {
	lang := GetLang()
	if m, ok := translations[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	// Fallback: Engels
	if m, ok := translations[LangEN]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	return key
}

// ──────────────────────────────────────────────────────────────────────────────
//  Vertalingen
// ──────────────────────────────────────────────────────────────────────────────

// translations bevat alle vertalingen, geïndexeerd op taalcode en sleutel.
var translations = map[string]map[string]string{

	// ── Engels ────────────────────────────────────────────────────────────────
	LangEN: {
		// Algemeen
		"unknown":  "unknown",
		"hostname": "Hostname",
		"version":  "Version",

		// Afsluittypen (voor weergave in UI)
		"type.shutdown":  "Shut down",
		"type.restart":   "Restart",
		"type.hibernate": "Hibernate",
		"type.sleep":     "Sleep",
		"type.logoff":    "Log off",

		// Systeemvak
		"tray.tooltip":      "HA Shutdown Client – %s (port %d)",
		"tray.info":         "ℹ  Connection info",
		"tray.info.tip":     "Show hostname, port and API key",
		"tray.settings":     "⚙  Settings...",
		"tray.settings.tip": "Change port, delay and shutdown type",
		"tray.about":        "ℹ  About...",
		"tray.about.tip":    "Show version and application information",
		"tray.language":     "🌐  Language",
		"tray.lang.en":      "English",
		"tray.lang.nl":      "Nederlands",
		"tray.quit":         "✕  Exit",
		"tray.quit.tip":     "Stop HA Shutdown Client",

		// Info-dialoog (systeemvak)
		"info.title": "HA Shutdown Client – Connection info",


		// Consoleopstart-banner
		"banner.title":   "  HA Windows Shutdown Client  v%s",
		"banner.host":    "  Hostname  : %s",
		"banner.port":    "  Port      : %d",
		"banner.apikey":  "  API key   : %s",
		"banner.type":    "  Shutdown  : %s",
		"banner.delay":   "  Delay     : %ds",
		"banner.hint":    "  Enter the API key when setting up Home Assistant.",
		"banner.server":  "API server started on port %d",
		"banner.mdns":    "mDNS advertisement active (auto-discovery enabled)",
		"banner.mdns.err":"Warning: mDNS not started: %v",
		"banner.notray":  "Press Ctrl+C to stop.",
		"banner.stopped": "Client stopped.",
		"banner.tray.exit":"System tray closed.",

		// CLI-uitvoer
		"cli.saved":          "✓ Default settings saved to the Windows registry.",
		"cli.saved.port":     "  Port      : %d",
		"cli.saved.delay":    "  Delay     : %ds",
		"cli.saved.type":     "  Shutdown  : %s",
		"cli.pw.new":         "✓ New API key generated:\n\n  %s\n",
		"cli.pw.hint":        "Enter this key when setting up the Home Assistant integration.",
		"cli.pw.label":       "API key   : %s",
		"cli.port.label":     "Port      : %d",
		"cli.delay.label":    "Delay     : %ds",
		"cli.type.label":     "Shutdown  : %s",
		"cli.wait":           "\nPress Enter to close...",

		// Help-scherm
		"help.text": `
HA Windows Shutdown Client v` + version + `
======================================
A lightweight HTTP API server that lets Home Assistant
shut down this Windows computer.

USAGE
  ha-shutdown-client.exe [options]

OPTIONS
  --port PORT         Listening port (default: 8765)
  --delay SECONDS     Default delay before shutdown (default: 30)
  --type TYPE         Default shutdown type (see below)
  --save-defaults     Save --port/--delay/--type to the registry
  --show-password     Show the API key and exit
  --reset-password    Generate a new API key
  --no-tray           Console mode (no system tray)
  --help              Show this help screen

SHUTDOWN TYPES
  shutdown    Shut down (default)
  restart     Restart
  hibernate   Hibernate (requires hiberfil.sys)
  sleep       Sleep
  logoff      Log off current user

EXAMPLES
  ha-shutdown-client.exe
  ha-shutdown-client.exe --show-password
  ha-shutdown-client.exe --delay 60 --type restart --save-defaults
  ha-shutdown-client.exe --reset-password
  ha-shutdown-client.exe --no-tray --port 9000

REGISTRY
  Settings are stored in:
  HKEY_CURRENT_USER\Software\HAShutdownClient
`,

		// Instellingendialoog – labels
		"dlg.port":        "Listening port:",
		"dlg.delay":       "Delay (seconds):",
		"dlg.type":        "Shutdown type:",
		"dlg.apikey":      "API key:",
		"dlg.btn.save":    "Save",
		"dlg.btn.cancel":  "Cancel",
		"dlg.btn.refresh": "Refresh",
		"dlg.btn.copy":    "Copy API key",
		"info.btn.copy":   "Copy API key to clipboard",
		"info.status.active": "Active",
		"info.copied":     "API key copied to clipboard.",

		// Instellingendialoog – validatiefouten
		"dlg.err.port":  "Port must be a whole number between 1 and 65535.\r\n\r\nRecommended range: 1024–65535 (no admin rights needed).",
		"dlg.err.delay": "Delay must be a whole number between 0 and 3600 seconds.",
		"dlg.err.type":  "Please select a shutdown type.",
		"dlg.err.title": "Validation error",

		// Instellingendialoog – opgeslagen
		"dlg.saved.title": "Saved",
		"dlg.saved.body":
			"✓ Settings saved to the Windows registry.\r\n\r\n" +
			"Port      : %d\r\n" +
			"Delay     : %d seconds\r\n" +
			"Shutdown  : %s\r\n\r\n" +
			"Restart the client to activate a new port.",

		// Wachtwoord vernieuwen
		"pw.reset.title": "New key generated",
		"pw.reset.body":
			"A new API key has been generated and\r\n" +
			"saved to the Windows registry.\r\n\r\n" +
			"New key:\r\n%s\r\n\r\n" +
			"Update this key in the\r\n" +
			"Home Assistant integration.",

		// Taalwijziging
		"lang.changed.title": "Language changed",
		"lang.changed.body":  "Language set to English.\r\nThe new language takes effect immediately.",
		// Autostart
		"dlg.group.connection": "Connection",
		"dlg.group.security":   "Security",
		"dlg.group.system":     "System",
		"dlg.autostart":        "Start automatically with Windows",
		"dlg.err.autostart":    "Could not update autostart setting:",
		"dlg.apikey.hint":      "API key for Home Assistant",
		"dlg.subtitle.settings": "Settings",
		"dlg.wintitle.settings": "HA Shutdown Client – Settings",

		// Info-dialoog
		"info.subtitle": "Connection info",

		// About-dialoog
		"about.wintitle":     "About HA Shutdown Client",
		"about.subtitle":     "About",
		"about.description":  "A lightweight HTTP API server that lets Home\nAssistant shut down this Windows computer.",
		"about.copyright":    "© 2024 – HA Shutdown Client contributors",
		"about.license":      "Released under the MIT License.",

		// Firewall
		"dlg.group.firewall":  "Windows Firewall",
		"dlg.fw.status":       "Rule status:",
		"dlg.fw.btn.create":   "Create rule (admin)",
		"dlg.fw.btn.refresh":  "Refresh",
		"fw.status.ok":        "✓  Inbound rule present",
		"fw.status.missing":   "✗  No inbound rule – connections may be blocked",
		"fw.status.unknown":   "?  Could not determine rule status",
		"fw.prompt.title":     "Windows Firewall",
		"fw.prompt.body":      "No Windows Firewall inbound rule was found for HA Shutdown Client (port %d).\n\nWithout this rule, Home Assistant cannot reach the client.\n\nCreate the firewall rule now?\n\n(Requires administrator rights – a UAC prompt will appear)\n\nChoose 'No' to skip and not be asked again.",
		"fw.ok.title":         "Firewall rule created",
		"fw.ok.body":          "The inbound firewall rule has been created successfully.\nHome Assistant can now connect to this client.",
		"fw.err.title":        "Firewall error",
		"fw.err.body":         "The firewall rule could not be created:\n%v\n\nYou can try again via Settings → Windows Firewall.",
		"fw.creating.body":    "The UAC prompt has been shown.\nIf you approved it, the firewall rule will be active within a few seconds.\n\nUse the 'Refresh' button to update the status.",

		// Privilege-status (weergegeven in info- en instellingendialoog)
		"priv.ok":      "✓  Shutdown permission: OK",
		"priv.denied":  "✗  Shutdown permission: missing — try running as administrator",
		"priv.unknown": "?  Shutdown permission: could not check",

		// Hostname-fallback
		"hostname.unknown": "unknown",
	},

	// ── Nederlands ────────────────────────────────────────────────────────────
	LangNL: {
		// Algemeen
		"unknown":  "onbekend",
		"hostname": "Hostnaam",
		"version":  "Versie",

		// Afsluittypen
		"type.shutdown":  "Afsluiten",
		"type.restart":   "Opnieuw opstarten",
		"type.hibernate": "Slaapstand",
		"type.sleep":     "Sluimerstand",
		"type.logoff":    "Afmelden",

		// Systeemvak
		"tray.tooltip":      "HA Shutdown Client – %s (poort %d)",
		"tray.info":         "ℹ  Verbindingsinfo tonen",
		"tray.info.tip":     "Toon hostnaam, poort en API-sleutel",
		"tray.settings":     "⚙  Instellingen...",
		"tray.settings.tip": "Pas poort, vertraging en afsluittype aan",
		"tray.about":        "ℹ  Over...",
		"tray.about.tip":    "Toon versie en applicatie-informatie",
		"tray.language":     "🌐  Taal",
		"tray.lang.en":      "English",
		"tray.lang.nl":      "Nederlands",
		"tray.quit":         "✕  Afsluiten",
		"tray.quit.tip":     "Stop de HA Shutdown Client",

		// Info-dialoog
		"info.title": "HA Shutdown Client – Verbindingsinfo",


		// Consoleopstart-banner
		"banner.title":    "  HA Windows Shutdown Client  v%s",
		"banner.host":     "  Hostnaam   : %s",
		"banner.port":     "  Poort      : %d",
		"banner.apikey":   "  API-sleutel: %s",
		"banner.type":     "  Afsluittype: %s",
		"banner.delay":    "  Vertraging : %ds",
		"banner.hint":     "  Voer de API-sleutel in bij het instellen van Home Assistant.",
		"banner.server":   "API-server gestart op poort %d",
		"banner.mdns":     "mDNS-advertentie actief (automatische ontdekking ingeschakeld)",
		"banner.mdns.err": "Waarschuwing: mDNS niet gestart: %v",
		"banner.notray":   "Druk op Ctrl+C om te stoppen.",
		"banner.stopped":  "Client gestopt.",
		"banner.tray.exit":"Systeemvak gesloten.",

		// CLI-uitvoer
		"cli.saved":       "✓ Standaardinstellingen opgeslagen in het Windows-register.",
		"cli.saved.port":  "  Poort      : %d",
		"cli.saved.delay": "  Vertraging : %ds",
		"cli.saved.type":  "  Afsluittype: %s",
		"cli.pw.new":      "✓ Nieuw API-wachtwoord gegenereerd:\n\n  %s\n",
		"cli.pw.hint":     "Voer dit wachtwoord in bij het instellen van Home Assistant.",
		"cli.pw.label":    "API-sleutel : %s",
		"cli.port.label":  "Poort       : %d",
		"cli.delay.label": "Vertraging  : %ds",
		"cli.type.label":  "Afsluittype : %s",
		"cli.wait":        "\nDruk op Enter om te sluiten...",

		// Help-scherm
		"help.text": `
HA Windows Shutdown Client v` + version + `
======================================
Een lichtgewicht HTTP-API-server waarmee Home Assistant
deze Windows-computer kan afsturen.

GEBRUIK
  ha-shutdown-client.exe [opties]

OPTIES
  --port POORT        Luisterpoort (standaard: 8765)
  --delay SECONDEN    Standaard vertraging voor afsluiten (standaard: 30)
  --type TYPE         Standaard afsluittype (zie hieronder)
  --save-defaults     Sla --port/--delay/--type op in het register
  --show-password     Toon het API-wachtwoord en sluit af
  --reset-password    Genereer een nieuw API-wachtwoord
  --no-tray           Consolemodus (geen systeemvak)
  --help              Dit scherm tonen

AFSLUITTYPEN
  shutdown    Afsluiten (standaard)
  restart     Opnieuw opstarten
  hibernate   Slaapstand (vereist hiberfil.sys)
  sleep       Sluimerstand
  logoff      Huidige gebruiker afmelden

VOORBEELDEN
  ha-shutdown-client.exe
  ha-shutdown-client.exe --show-password
  ha-shutdown-client.exe --delay 60 --type restart --save-defaults
  ha-shutdown-client.exe --reset-password
  ha-shutdown-client.exe --no-tray --port 9000

REGISTER
  Instellingen worden opgeslagen in:
  HKEY_CURRENT_USER\Software\HAShutdownClient
`,

		// Instellingendialoog – labels
		"dlg.port":        "Luisterpoort:",
		"dlg.delay":       "Vertraging (seconden):",
		"dlg.type":        "Afsluittype:",
		"dlg.apikey":      "API-sleutel:",
		"dlg.btn.save":    "Opslaan",
		"dlg.btn.cancel":  "Annuleren",
		"dlg.btn.refresh": "Vernieuwen",
		"dlg.btn.copy":    "Kopieer sleutel",
		"info.btn.copy":   "Kopieer API-sleutel",
		"info.status.active": "Actief",
		"info.copied":     "API-sleutel gekopieerd naar klembord.",

		// Instellingendialoog – validatiefouten
		"dlg.err.port":  "Poort moet een geheel getal zijn tussen 1 en 65535.\r\n\r\nAanbevolen bereik: 1024–65535 (geen beheerdersrechten nodig).",
		"dlg.err.delay": "Vertraging moet een geheel getal zijn tussen 0 en 3600 seconden.",
		"dlg.err.type":  "Selecteer een afsluittype.",
		"dlg.err.title": "Validatiefout",

		// Instellingendialoog – opgeslagen
		"dlg.saved.title": "Opgeslagen",
		"dlg.saved.body":
			"✓ Instellingen opgeslagen in het Windows-register.\r\n\r\n" +
			"Poort      : %d\r\n" +
			"Vertraging : %d seconden\r\n" +
			"Afsluittype: %s\r\n\r\n" +
			"Herstart de client om de nieuwe poort te activeren.",

		// Wachtwoord vernieuwen
		"pw.reset.title": "Nieuw wachtwoord aangemaakt",
		"pw.reset.body":
			"Er is een nieuw API-wachtwoord aangemaakt en\r\n" +
			"opgeslagen in het Windows-register.\r\n\r\n" +
			"Nieuw wachtwoord:\r\n%s\r\n\r\n" +
			"Pas dit wachtwoord aan in de\r\n" +
			"Home Assistant-integratie.",

		// Taalwijziging
		"lang.changed.title": "Taal gewijzigd",
		"lang.changed.body":  "Taal ingesteld op Nederlands.\r\nDe nieuwe taal is direct actief.",
		// Autostart
		"dlg.group.connection": "Verbinding",
		"dlg.group.security":   "Beveiliging",
		"dlg.group.system":     "Systeem",
		"dlg.autostart":        "Automatisch starten met Windows",
		"dlg.err.autostart":    "Autostart-instelling kon niet worden bijgewerkt:",
		"dlg.apikey.hint":      "API-sleutel voor Home Assistant",
		"dlg.subtitle.settings": "Instellingen",
		"dlg.wintitle.settings": "HA Shutdown Client – Instellingen",

		// Info-dialoog
		"info.subtitle": "Verbindingsinfo",

		// About-dialoog
		"about.wintitle":     "Over HA Shutdown Client",
		"about.subtitle":     "Over",
		"about.description":  "Een lichtgewicht HTTP-API-server waarmee Home\nAssistant deze Windows-computer kan afsturen.",
		"about.copyright":    "© 2024 – HA Shutdown Client bijdragers",
		"about.license":      "Uitgebracht onder de MIT-licentie.",

		// Firewall
		"dlg.group.firewall":  "Windows Firewall",
		"dlg.fw.status":       "Regelstatus:",
		"dlg.fw.btn.create":   "Regel aanmaken (admin)",
		"dlg.fw.btn.refresh":  "Vernieuwen",
		"fw.status.ok":        "✓  Inbound-regel aanwezig",
		"fw.status.missing":   "✗  Geen inbound-regel – verbindingen kunnen geblokkeerd zijn",
		"fw.status.unknown":   "?  Regelstatus niet te bepalen",
		"fw.prompt.title":     "Windows Firewall",
		"fw.prompt.body":      "Er is geen Windows Firewall-inbound-regel gevonden voor HA Shutdown Client (poort %d).\n\nZonder deze regel kan Home Assistant de client niet bereiken.\n\nFirewallregel nu aanmaken?\n\n(Vereist beheerdersrechten – een UAC-melding verschijnt)\n\nKies 'Nee' om dit over te slaan en niet meer te vragen.",
		"fw.ok.title":         "Firewallregel aangemaakt",
		"fw.ok.body":          "De inbound-firewallregel is succesvol aangemaakt.\nHome Assistant kan nu verbinding maken met deze client.",
		"fw.err.title":        "Firewall-fout",
		"fw.err.body":         "De firewallregel kon niet worden aangemaakt:\n%v\n\nProbeer het opnieuw via Instellingen → Windows Firewall.",
		"fw.creating.body":    "De UAC-melding is getoond.\nAls u die hebt goedgekeurd, is de firewallregel binnen enkele seconden actief.\n\nGebruik de knop 'Vernieuwen' om de status bij te werken.",

		// Privilege-status
		"priv.ok":      "✓  Afsluitrechten: aanwezig",
		"priv.denied":  "✗  Afsluitrechten: ontbreken — probeer als beheerder uit te voeren",
		"priv.unknown": "?  Afsluitrechten: niet te controleren",

		// Hostname-fallback
		"hostname.unknown": "onbekend",
	},
}


