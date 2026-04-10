//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getlantern/systray"
)

const version = "1.0.0"

func main() {
	// ── Vlag-definities ──────────────────────────────────────────────────────
	flagPort         := flag.Int("port", 0, "Listening port (default: 8765)")
	flagDelay        := flag.Int("delay", 0, "Default delay in seconds (default: 30)")
	flagType         := flag.String("type", "", "Shutdown type: shutdown|restart|hibernate|sleep|logoff")
	flagSaveDefaults := flag.Bool("save-defaults", false, "Save --port/--delay/--type to registry")
	flagShowPW       := flag.Bool("show-password", false, "Show the API key and exit")
	flagResetPW      := flag.Bool("reset-password", false, "Generate a new API key")
	flagNoTray       := flag.Bool("no-tray", false, "Console mode (no system tray)")
	flagHelp         := flag.Bool("help", false, "Show this help screen")
	flag.Usage = func() { /* suppress default; we handle --help explicitly */ }
	flag.Parse()

	// ── Taal laden voor alle overige initialisatie ────────────────────────────
	LoadLang()

	// ── Bepaal of we een console nodig hebben ────────────────────────────────
	needsConsole := *flagShowPW || *flagResetPW || *flagSaveDefaults ||
		*flagNoTray || *flagHelp

	var newConsoleWindow bool
	if needsConsole {
		newConsoleWindow = ensureConsole()
		log.SetFlags(log.Ltime)
	}

	// ── Helpscherm ───────────────────────────────────────────────────────────
	if *flagHelp {
		fmt.Fprint(os.Stdout, T("help.text"))
		waitIfNewWindow(newConsoleWindow)
		return
	}

	// ── Config laden en CLI-overschrijvingen toepassen ────────────────────────
	cfg := loadConfig()
	if *flagPort != 0 {
		cfg.Port = *flagPort
	}
	if *flagDelay != 0 {
		cfg.Delay = *flagDelay
	}
	if *flagType != "" {
		// Valideer het opgegeven afsluittype vóór gebruik.
		// Zo wordt voorkomen dat een ongeldige waarde in het register wordt opgeslagen.
		if !isValidShutdownType(*flagType) {
			fmt.Fprintf(os.Stderr, "Ongeldig afsluittype %q. Geldige waarden: %s\n",
				*flagType, strings.Join(typeKeys, ", "))
			os.Exit(1)
		}
		cfg.ShutdownType = *flagType
	}

	// ── Sub-opdrachten die meteen afsluiten ───────────────────────────────────
	if *flagSaveDefaults {
		cfg.Save()
		fmt.Fprintln(os.Stdout, T("cli.saved"))
		fmt.Fprintf(os.Stdout, T("cli.saved.port")+"\n", cfg.Port)
		fmt.Fprintf(os.Stdout, T("cli.saved.delay")+"\n", cfg.Delay)
		fmt.Fprintf(os.Stdout, T("cli.saved.type")+"\n", cfg.ShutdownType)
		waitIfNewWindow(newConsoleWindow)
		return
	}

	if *flagResetPW {
		pw := refreshActivePassword()
		fmt.Fprintf(os.Stdout, T("cli.pw.new"), pw)
		fmt.Fprintln(os.Stdout, T("cli.pw.hint"))
		waitIfNewWindow(newConsoleWindow)
		return
	}

	pw := initActivePassword()

	if *flagShowPW {
		fmt.Fprintf(os.Stdout, T("cli.pw.label")+"\n", pw)
		fmt.Fprintf(os.Stdout, T("cli.port.label")+"\n", cfg.Port)
		fmt.Fprintf(os.Stdout, T("cli.delay.label")+"\n", cfg.Delay)
		fmt.Fprintf(os.Stdout, T("cli.type.label")+"\n", cfg.ShutdownType)
		waitIfNewWindow(newConsoleWindow)
		return
	}

	// ── Startbanner ──────────────────────────────────────────────────────────
	sep := strings.Repeat("─", 52)
	log.Println(sep)
	log.Printf(T("banner.title"), version)
	log.Println(sep)
	log.Printf(T("banner.host"), mustHostname())
	log.Printf(T("banner.port"), cfg.Port)
	log.Printf(T("banner.apikey"), pw)
	log.Printf(T("banner.type"), cfg.ShutdownType)
	log.Printf(T("banner.delay"), cfg.Delay)
	log.Println(sep)
	log.Println(T("banner.hint"))
	log.Println(sep)

	// ── HTTP-server starten ───────────────────────────────────────────────────
	srv := newServer(cfg)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()
	log.Printf(T("banner.server"), cfg.Port)

	// ── mDNS-advertentie starten ──────────────────────────────────────────────
	mdns, err := startMDNS(cfg.Port)
	if err != nil {
		log.Printf(T("banner.mdns.err"), err)
	} else {
		defer mdns.Shutdown()
		log.Println(T("banner.mdns"))
	}

	// ── Systeemvak of consolemodus ────────────────────────────────────────────
	if *flagNoTray {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		log.Println(T("banner.notray"))
		<-quit
		// Verwijder de kanaalregistratie zodat de runtime de goroutine kan opruimen.
		signal.Stop(quit)
	} else {
		systray.Run(
			func() { onTrayReady(cfg) },
			func() { log.Println(T("banner.tray.exit")) },
		)
	}

	// ── Nette HTTP-server afsluiting ─────────────────────────────────────────
	// Geef lopende requests 5 seconden om te voltooien vóór we geforceerd stoppen.
	// cancel() direct aanroepen (niet via defer) zodat de context precies na
	// srv.Shutdown vrijkomt en niet via de defer-stack bij een eerdere panic.
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("HTTP server afsluiten: %v", err)
	}
	cancel()

	log.Println(T("banner.stopped"))
}

// isValidShutdownType controleert of de opgegeven afsluittype-string geldig is.
// Gebruikt de validShutdownTypes-map voor O(1) opzoeken.
func isValidShutdownType(t string) bool {
	_, ok := validShutdownTypes[t]
	return ok
}

// mustHostname geeft de hostnaam terug, of de vertaling van "unknown" bij een fout.
// De waarde wordt bij opstart gecachet in server.go; deze functie doet geen syscall.
func mustHostname() string {
	if cachedHostname != "" {
		return cachedHostname
	}
	return T("hostname.unknown")
}

// waitIfNewWindow wacht op Enter als het programma een nieuw consolevenster
// heeft geopend (dubbelklik). Zo sluit het venster niet meteen.
func waitIfNewWindow(newWindow bool) {
	if !newWindow {
		return
	}
	fmt.Fprint(os.Stdout, T("cli.wait"))
	buf := make([]byte, 1)
	os.Stdin.Read(buf) //nolint:errcheck
}
