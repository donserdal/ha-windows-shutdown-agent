//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

// ── Privilege-controle ────────────────────────────────────────────────────────

var (
	privOnce    sync.Once
	privAllowed bool
	privErr     error
)

// getPrivilegeStatus geeft terug of het proces SeShutdownPrivilege heeft.
// Het resultaat wordt gecachet via sync.Once: de controle wordt maximaal
// één keer per sessie uitgevoerd, omdat privileges procesgebonden zijn en
// niet veranderen tijdens de looptijd.
func getPrivilegeStatus() (allowed bool, err error) {
	privOnce.Do(func() {
		privAllowed, privErr = checkShutdownPrivilege()
	})
	return privAllowed, privErr
}

// checkShutdownPrivilege controleert via de Windows-tokenAPI of het huidige
// proces SeShutdownPrivilege heeft.
//
// Dit privilege is vereist voor:
//   - shutdown.exe /s  (afsluiten)
//   - shutdown.exe /r  (opnieuw opstarten)
//   - shutdown.exe /h  (slaapstand)
//   - shutdown.exe /l  (afmelden)
//
// Aanwezig bij:  normale gebruikers, administrators, SYSTEM.
// Afwezig bij:   sterk beperkte service-accounts (LOCAL SERVICE, NETWORK SERVICE).
func checkShutdownPrivilege() (bool, error) {
	// Open het token van het huidige proces voor leestoegang.
	var token windows.Token
	if err := windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_QUERY,
		&token,
	); err != nil {
		return false, fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer token.Close()

	// Zoek de LUID op voor "SeShutdownPrivilege".
	var luid windows.LUID
	name, err := windows.UTF16PtrFromString("SeShutdownPrivilege")
	if err != nil {
		return false, fmt.Errorf("UTF16Ptr: %w", err)
	}
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return false, fmt.Errorf("LookupPrivilegeValue: %w", err)
	}

	// Vraag de buffergrootte op die nodig is voor de TOKEN_PRIVILEGES structuur.
	var needed uint32
	_ = windows.GetTokenInformation(token, windows.TokenPrivileges, nil, 0, &needed)
	if needed == 0 {
		return false, fmt.Errorf("GetTokenInformation: onverwachte lege buffer")
	}

	buf := make([]byte, needed)
	if err := windows.GetTokenInformation(
		token, windows.TokenPrivileges, &buf[0], uint32(len(buf)), &needed,
	); err != nil {
		return false, fmt.Errorf("GetTokenInformation: %w", err)
	}

	// Lees het TOKEN_PRIVILEGES-blok uit de byte-buffer via encoding/binary.
	//
	// Structuurindeling (Windows API, little-endian):
	//   [0..3]   PrivilegeCount  (uint32)
	//   [4..]    LUID_AND_ATTRIBUTES[] – elk element 12 bytes:
	//              [0..3]  LUID.LowPart   (uint32)
	//              [4..7]  LUID.HighPart  (int32, gelezen als uint32)
	//              [8..11] Attributes     (uint32)
	//
	// encoding/binary vermijdt unsafe.Pointer reinterpret casts en is go vet-schoon.
	const (
		countSize       = 4  // sizeof(uint32)
		luidAndAttrSize = 12 // sizeof(LUID_AND_ATTRIBUTES)
	)

	if len(buf) < countSize {
		return false, fmt.Errorf("buffer te klein voor PrivilegeCount")
	}
	count := binary.LittleEndian.Uint32(buf[0:4])

	for i := uint32(0); i < count; i++ {
		offset := countSize + int(i)*luidAndAttrSize
		if offset+8 > len(buf) {
			break // bounds-bewaking: corrupte buffer
		}
		lowPart := binary.LittleEndian.Uint32(buf[offset : offset+4])
		highPart := int32(binary.LittleEndian.Uint32(buf[offset+4 : offset+8]))

		if lowPart == luid.LowPart && highPart == luid.HighPart {
			return true, nil // privilege aanwezig in het token
		}
	}
	return false, nil
}
