//go:build windows

package main

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ── Firewallbeheer ────────────────────────────────────────────────────────────
//
// De client luistert op een TCP-poort. Windows Defender Firewall blokkeert
// inkomende verbindingen tenzij er een expliciete inbound-regel is.
//
// Strategie:
//   - Controleer via netsh (lees, geen elevatie nodig)
//   - Maak aan via ShellExecuteW met runas (UAC-prompt, eenmalig)
//   - Sla op in het register of de gebruiker "niet meer vragen" heeft gekozen

const firewallRuleName = "HA Shutdown Client"

// ── Statuscache ───────────────────────────────────────────────────────────────

var (
	fwMu     sync.Mutex
	fwCached *bool // nil = nog niet gecontroleerd
)

// FirewallStatus beschrijft de huidige staat van de firewallregel.
type FirewallStatus int

const (
	FirewallUnknown FirewallStatus = iota
	FirewallRulePresent
	FirewallRuleMissing
)

// getFirewallStatus geeft de status van de firewallregel terug.
// Resultaat wordt gecachet totdat refreshFirewallCache() wordt aangeroepen.
//
// De mutex wordt NIET vastgehouden tijdens de netsh-aanroep (die kan seconden
// duren). In plaats daarvan wordt het resultaat pas opgeslagen nadat de check
// klaar is, met een korte lock alleen voor lezen/schrijven van de pointer.
func getFirewallStatus() FirewallStatus {
	fwMu.Lock()
	cached := fwCached
	fwMu.Unlock()

	if cached != nil {
		if *cached {
			return FirewallRulePresent
		}
		return FirewallRuleMissing
	}

	// Voer de controle uit zonder lock (kan traag zijn).
	ok, err := isFirewallRulePresent()
	if err != nil {
		log.Printf("Firewallcontrole mislukt: %v", err)
		return FirewallUnknown
	}

	// Sla op met lock; een mogelijke race met refreshFirewallCache is onschadelijk:
	// het slechtste geval is dat we een iets verouderd resultaat cachen.
	fwMu.Lock()
	if fwCached == nil { // sla alleen op als de cache nog leeg is
		fwCached = &ok
	}
	fwMu.Unlock()

	if ok {
		return FirewallRulePresent
	}
	return FirewallRuleMissing
}

// refreshFirewallCache wist de cache zodat de volgende aanroep van
// getFirewallStatus opnieuw controleert.
func refreshFirewallCache() {
	fwMu.Lock()
	fwCached = nil
	fwMu.Unlock()
}

// ── Controleer of de regel aanwezig is ───────────────────────────────────────

// isFirewallRulePresent controleert via netsh of er al een inbound-regel
// bestaat met de naam firewallRuleName.
// Vereist geen beheerdersrechten.
func isFirewallRulePresent() (bool, error) {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule",
		"name="+firewallRuleName, "dir=in")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		// netsh geeft exit code 1 als er geen regels gevonden zijn; dat is geen fout.
		// Elke andere fout (bijv. netsh niet gevonden) is wel een echte fout.
		if _, ok := err.(*exec.ExitError); ok {
			// Exit 1 = "No rules match the specified criteria." → gewoon geen regel.
			return false, nil
		}
		return false, fmt.Errorf("netsh uitvoeren mislukt: %w", err)
	}
	return strings.Contains(string(out), firewallRuleName), nil
}

// ── Maak de regel aan (met UAC-elevatie) ─────────────────────────────────────

// createFirewallRule maakt een inbound TCP-firewallregel aan voor de opgegeven
// poort. Omdat schrijven naar de firewallconfiguratie beheerdersrechten vereist,
// wordt het netsh-commando uitgevoerd via ShellExecuteW met het "runas"-verb.
// Dit toont een UAC-prompt aan de gebruiker.
//
// Geeft nil terug als de UAC-prompt geaccepteerd is en netsh gestart is.
// Of de regel daadwerkelijk aangemaakt is, moet achteraf worden gecontroleerd
// via refreshFirewallCache() + getFirewallStatus().
func createFirewallRule(port int) error {
	args := fmt.Sprintf(
		`advfirewall firewall add rule name="%s" dir=in action=allow protocol=TCP localport=%d enable=yes profile=any description="Home Assistant Windows Shutdown Client HTTP API"`,
		firewallRuleName, port,
	)
	return shellExecuteElevated("netsh", args)
}

// ── ShellExecuteW (runas) ─────────────────────────────────────────────────────

// shell32dll gebruikt windows.NewLazySystemDLL in plaats van syscall.NewLazyDLL.
// NewLazySystemDLL zoekt uitsluitend in de Windows System32-map, waardoor
// DLL-hijacking vanuit de huidige werkmap onmogelijk is.
var (
	shell32dll        = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = shell32dll.NewProc("ShellExecuteW")
)

// shellExecuteElevated start een programma met verhoogde rechten via UAC.
// Gebruikt ShellExecuteW met verb "runas" zodat Windows de UAC-prompt toont.
//
// De functie wacht NIET tot het child-proces klaar is; gebruik daarna
// refreshFirewallCache() om de nieuwe status op te vragen.
func shellExecuteElevated(program, params string) error {
	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("UTF16Ptr verb: %w", err)
	}
	progPtr, err := syscall.UTF16PtrFromString(program)
	if err != nil {
		return fmt.Errorf("UTF16Ptr program: %w", err)
	}
	paramsPtr, err := syscall.UTF16PtrFromString(params)
	if err != nil {
		return fmt.Errorf("UTF16Ptr params: %w", err)
	}

	// De uintptr(unsafe.Pointer(x))-conversies staan inline in de Call zodat de
	// Go-compiler garandeert dat de pointers geldig zijn tijdens de syscall.
	// Dit is het correcte patroon zoals beschreven in de unsafe-documentatie.
	const swShownormal = 1
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),   //nolint:unsafeptr
		uintptr(unsafe.Pointer(progPtr)),   //nolint:unsafeptr
		uintptr(unsafe.Pointer(paramsPtr)), //nolint:unsafeptr
		0,
		swShownormal,
	)

	// ShellExecuteW geeft een waarde > 32 terug bij succes.
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW mislukt (code %d)", ret)
	}
	return nil
}

// ── Register: "niet meer vragen" ─────────────────────────────────────────────

const regFirewallPrompted = "FirewallPrompted"

// firewallPromptDone geeft terug of de gebruiker al eerder heeft gekozen om
// niet meer gevraagd te worden over de firewallregel.
func firewallPromptDone() bool {
	return regGetString(regFirewallPrompted, "") == "1"
}

// setFirewallPromptDone slaat op dat de gebruiker de prompt heeft gezien.
func setFirewallPromptDone() {
	_ = regSetString(regFirewallPrompted, "1")
}
