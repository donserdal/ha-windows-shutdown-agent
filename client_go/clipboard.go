//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	procOpenClipboard    = user32dll.NewProc("OpenClipboard")
	procCloseClipboard   = user32dll.NewProc("CloseClipboard")
	procEmptyClipboard   = user32dll.NewProc("EmptyClipboard")
	procSetClipboardData = user32dll.NewProc("SetClipboardData")

	procGlobalAlloc  = kernel32dll.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32dll.NewProc("GlobalFree")
	procGlobalLock   = kernel32dll.NewProc("GlobalLock")
	procGlobalUnlock = kernel32dll.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13      // CF_UNICODETEXT
	gmemMoveable  = 0x0002  // GMEM_MOVEABLE
)

// copyToClipboard plaatst de gegeven tekst als CF_UNICODETEXT op het Windows-klembord.
// Thread-safe: OpenClipboard blokkeert totdat het klembord vrijkomt.
func copyToClipboard(text string) error {
	// Converteer naar UTF-16 (Windows Unicode-formaat)
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return fmt.Errorf("UTF-16 conversie mislukt: %w", err)
	}
	byteLen := uintptr(len(utf16) * 2) // 2 bytes per uint16

	// Alloceer verplaatsbaar geheugen (vereist door SetClipboardData).
	// Belangrijk: na een succesvolle SetClipboardData is het klembord
	// eigenaar van hMem en mag GlobalFree NIET worden aangeroepen.
	// Bij elke mislukte stap daarvóór moet hMem wél worden vrijgegeven.
	hMem, _, err := procGlobalAlloc.Call(gmemMoveable, byteLen)
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc mislukt: %w", err)
	}

	// Vergrendel het geheugenblok en kopieer de tekst erin.
	ptr, _, err := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return fmt.Errorf("GlobalLock mislukt: %w", err)
	}
	// ptr is Windows-heap geheugen (GlobalAlloc), niet Go-heap.
	// De Go GC beheert dit blok niet en zal het nooit verplaatsen.
	// De uintptr→unsafe.Pointer conversie is hier altijd geldig.
	// go vet -unsafeptr geeft hier een false positive; zie build.bat.
	dst := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(utf16)) //nolint:unsafeptr
	copy(dst, utf16)
	procGlobalUnlock.Call(hMem)

	// Open klembord, leeg het en stel de data in.
	r, _, err := procOpenClipboard.Call(0)
	if r == 0 {
		procGlobalFree.Call(hMem) // klembord niet open → eigenaar nog steeds wij
		return fmt.Errorf("OpenClipboard mislukt: %w", err)
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	r, _, err = procSetClipboardData.Call(cfUnicodeText, hMem)
	if r == 0 {
		// SetClipboardData mislukt → klembord bezit hMem NIET → wij moeten vrijgeven.
		procGlobalFree.Call(hMem)
		return fmt.Errorf("SetClipboardData mislukt: %w", err)
	}
	// Succes: het klembord bezit hMem nu; wij mogen het niet meer vrijgeven.
	return nil
}
