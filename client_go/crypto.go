//go:build windows

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
)

// ── AES-256-GCM versleuteling ─────────────────────────────────────────────────
//
// De AES-sleutel wordt automatisch gegenereerd bij de eerste start en opgeslagen
// als base64-string in HKCU\Software\HAShutdownClient (sleutel: "AESKey").
// Het API-wachtwoord wordt versleuteld opgeslagen als "EncryptedPassword".
//
// Formaat van de versleutelde waarde: base64(nonce || ciphertext)
// waarbij nonce 12 bytes is (GCM-standaard).

// ── AES-sleutel cache ─────────────────────────────────────────────────────────
//
// De AES-sleutel verandert niet tijdens een sessie. We cachen hem eenmalig
// via sync.Once zodat er nooit een TOCTOU-race kan optreden tussen twee
// goroutines die beide nil zien en beide proberen een sleutel te genereren.

var aesKeyOnce sync.Once
var aesKeyCached []byte
var aesKeyErr error

// getOrCreateAESKey geeft de bestaande AES-sleutel terug of genereert een nieuwe.
// Thread-safe via sync.Once — de initialisatie vindt maximaal één keer plaats.
func getOrCreateAESKey() ([]byte, error) {
	aesKeyOnce.Do(func() {
		aesKeyCached, aesKeyErr = loadOrGenerateAESKey()
	})
	return aesKeyCached, aesKeyErr
}

// loadOrGenerateAESKey bevat de werkelijke laad/genereer-logica voor de AES-sleutel.
func loadOrGenerateAESKey() ([]byte, error) {
	keyB64 := regGetString("AESKey", "")
	if keyB64 != "" {
		key, err := base64.StdEncoding.DecodeString(keyB64)
		if err == nil && len(key) == 32 {
			return key, nil
		}
		// Ongeldige sleutel → verwijderen en opnieuw genereren
		_ = regSetString("AESKey", "")
	}

	// Nieuwe 256-bits sleutel genereren
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("AES-sleutel genereren mislukt: %w", err)
	}
	if err := regSetString("AESKey", base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("AES-sleutel opslaan mislukt: %w", err)
	}
	return key, nil
}

// encryptString versleutelt een tekst met AES-256-GCM.
// Geeft een base64-gecodeerde string terug (nonce || ciphertext).
func encryptString(plaintext string) (string, error) {
	key, err := getOrCreateAESKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("AES-cipher aanmaken mislukt: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("GCM aanmaken mislukt: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce genereren mislukt: %w", err)
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// decryptString ontsleutelt een base64-gecodeerde AES-256-GCM-waarde.
func decryptString(encB64 string) (string, error) {
	key, err := getOrCreateAESKey()
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(encB64)
	if err != nil {
		return "", fmt.Errorf("base64-decoderen mislukt: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("AES-cipher aanmaken mislukt: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("GCM aanmaken mislukt: %w", err)
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", fmt.Errorf("versleutelde data te kort")
	}
	plain, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("ontsleutelen mislukt (verkeerde sleutel?): %w", err)
	}
	return string(plain), nil
}

// ── Wachtwoordbeheer ─────────────────────────────────────────────────────────

// getAPIPassword geeft het huidige API-wachtwoord terug.
// Als er nog geen wachtwoord bestaat (eerste start), wordt er automatisch
// een willekeurig wachtwoord aangemaakt en versleuteld opgeslagen.
func getAPIPassword() string {
	enc := regGetString("EncryptedPassword", "")
	if enc != "" {
		pw, err := decryptString(enc)
		if err == nil && pw != "" {
			return pw
		}
		// Ongeldig of beschadigd → verwijderen en opnieuw genereren
		log.Printf("Waarschuwing: opgeslagen wachtwoord ongeldig (%v). Nieuw wachtwoord genereren.", err)
		_ = regSetString("EncryptedPassword", "")
	}
	return generateAndStorePassword()
}

// refreshActivePassword genereert een nieuw wachtwoord, slaat het op in het
// register en werkt de in-memory cache direct bij.
// Geeft het nieuwe wachtwoord terug.
func refreshActivePassword() string {
	pw := generateAndStorePassword() // nieuw wachtwoord → register
	activePassword.Store(pw)         // → in-memory cache (direct actief voor de server)
	log.Printf("Actief API-wachtwoord bijgewerkt (cache + register)")
	return pw
}

// generateAndStorePassword genereert een cryptografisch veilig wachtwoord
// van 32 willekeurige bytes (≈43 tekens in base64url), versleutelt het en
// slaat het op in het register.
func generateAndStorePassword() string {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		log.Fatalf("Wachtwoord genereren mislukt: %v", err)
	}
	pw := base64.RawURLEncoding.EncodeToString(raw)

	enc, err := encryptString(pw)
	if err != nil {
		log.Fatalf("Wachtwoord versleutelen mislukt: %v", err)
	}
	if err := regSetString("EncryptedPassword", enc); err != nil {
		log.Fatalf("Wachtwoord opslaan mislukt: %v", err)
	}

	log.Printf("Nieuw API-wachtwoord gegenereerd en versleuteld opgeslagen")
	return pw
}

// ── Live-wachtwoord cache ─────────────────────────────────────────────────────
//
// activePassword slaat het actieve wachtwoord op in het geheugen zodat:
//  1. De HTTP-server het wachtwoord niet bij elk verzoek uit het register hoeft
//     te lezen.
//  2. Een wachtwoord-reset via het instellingendialoog direct effect heeft
//     zonder herstart van de client.
//
// sync/atomic.Value garandeert dat reads en writes vanuit meerdere goroutines
// (HTTP-server, UI-thread) altijd consistent zijn.

var activePassword atomic.Value // slaat een string op

// initActivePassword laadt het huidige wachtwoord in de cache.
// Moet éénmalig worden aangeroepen vóór het starten van de HTTP-server.
func initActivePassword() string {
	pw := getAPIPassword()
	activePassword.Store(pw)
	return pw
}

// getCurrentPassword geeft het actieve wachtwoord terug uit de in-memory cache.
// Thread-safe; geen registry-toegang.
func getCurrentPassword() string {
	if v, ok := activePassword.Load().(string); ok && v != "" {
		return v
	}
	// Fallback: lees opnieuw uit register (zou normaal niet nodig moeten zijn)
	pw := getAPIPassword()
	activePassword.Store(pw)
	return pw
}


