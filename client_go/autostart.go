//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// HKCU\Software\Microsoft\Windows\CurrentVersion\Run
// Schrijven/lezen hiervan vereist GEEN beheerdersrechten.
// De entries in deze sleutel worden uitgevoerd zodra de huidige gebruiker
// zich aanmeldt bij Windows.
const (
	runKeyPath     = `Software\Microsoft\Windows\CurrentVersion\Run`
	autoStartEntry = "HAShutdownClient"
)

var procGetModuleFileNameW = kernel32dll.NewProc("GetModuleFileNameW")

// getExePath geeft het volledige pad van de huidige executable terug.
// Gebruikt GetModuleFileNameW met een grote buffer voor lange UNC-paden.
func getExePath() (string, error) {
	buf := make([]uint16, 32768)
	n, _, err := procGetModuleFileNameW.Call(
		0, // NULL = huidig proces
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if n == 0 {
		return "", fmt.Errorf("GetModuleFileNameW: %w", err)
	}
	return syscall.UTF16ToString(buf[:n]), nil
}

// isAutoStartEnabled controleert of de autostart-entry aanwezig is.
func isAutoStartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(autoStartEntry)
	return err == nil
}

// setAutoStart schrijft of verwijdert de autostart-entry in HKCU\Run.
// Aanhalingstekens rondom het pad voorkomen problemen bij spaties in het pad.
func setAutoStart(enable bool) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		runKeyPath,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("Run-sleutel openen mislukt: %w", err)
	}
	defer k.Close()

	if !enable {
		err = k.DeleteValue(autoStartEntry)
		if err != nil && err != registry.ErrNotExist {
			return fmt.Errorf("autostart-entry verwijderen mislukt: %w", err)
		}
		return nil
	}

	exePath, err := getExePath()
	if err != nil {
		return fmt.Errorf("exe-pad bepalen mislukt: %w", err)
	}
	// Gebruik --no-tray NIET standaard: de gebruiker wil het systeem-vak.
	// Aanhalingstekens zijn verplicht voor paden met spaties.
	return k.SetStringValue(autoStartEntry, `"`+exePath+`"`)
}
