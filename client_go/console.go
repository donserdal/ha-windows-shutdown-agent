//go:build windows

package main

import (
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	// windows.NewLazySystemDLL zoekt uitsluitend in System32 — geen CWD-risico.
	kernel32dll       = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32dll.NewProc("AttachConsole")
	procAllocConsole  = kernel32dll.NewProc("AllocConsole")
	procSetConTitle   = kernel32dll.NewProc("SetConsoleTitleW")
)

const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS = (DWORD)-1

// ensureConsole zorgt ervoor dat het proces een consolevenster heeft en dat
// os.Stdout / os.Stderr / log naar dat venster schrijven.
//
// Strategie:
//  1. Probeer de console van het parent-proces over te nemen (cmd.exe,
//     PowerShell, Windows Terminal). Als dat lukt, schrijven we een lege
//     regel zodat uitvoer NIET overlapt met de al-afgedrukte prompt.
//  2. Als er geen parent-console is (dubbelklik in Verkenner), maak dan een
//     nieuw consolevenster aan.
//
// Geeft true terug als een NIEUW venster aangemaakt is.
func ensureConsole() (newWindow bool) {
	// Stap 1: probeer de parent-console over te nemen
	r, _, _ := procAttachConsole.Call(attachParentProcess)
	if r != 0 {
		rewireStdio()
		// ── OVERLAP-FIX ────────────────────────────────────────────────────────
		// Nadat cmd.exe of PowerShell een GUI-proces start, staat de cursor al
		// aan het einde van de prompt-regel (bijv. "C:\> "). Als wij dan
		// direct schrijven, overlapt onze eerste regel met die prompt.
		//
		// \r beweegt de cursor naar kolom 0; de volgende schrijfactie
		// overschrijft de prompt-tekst. De extra \n zorgt voor een lege
		// tussenregel zodat de uitvoer goed van de prompt gescheiden blijft.
		fmt.Fprint(os.Stdout, "\r\n")
		return false
	}

	// Stap 2: maak een nieuw consolevenster aan
	procAllocConsole.Call()
	rewireStdio()

	// Stel een herkenbare venstertitel in
	title, _ := syscall.UTF16PtrFromString("HA Shutdown Client")
	procSetConTitle.Call(uintptr(unsafe.Pointer(title)))

	return true
}

// rewireStdio heropent CONOUT$ / CONIN$ en koppelt os.Stdout, os.Stderr,
// os.Stdin en log eraan.
//
// Na (Attach|Alloc)Console zijn de Go-runtime file-handles nog steeds
// ongeldig (ze werden bij opstart van de GUI-binary gesloten). Door CONOUT$
// opnieuw te openen krijgen we werkende file-descriptors.
func rewireStdio() {
	if conout, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = conout
		os.Stderr = conout
		log.SetOutput(conout)
	}
	if conin, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = conin
	}
	log.SetFlags(log.Ltime)
}
