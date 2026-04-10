//go:build windows

package main

import (
	"fmt"
	"strconv"

	"golang.org/x/sys/windows/registry"
)

// Registerpad: HKEY_CURRENT_USER\Software\HAShutdownClient
// Geen beheerdersrechten nodig – HKCU is altijd schrijfbaar voor de huidige gebruiker.
const regPath = `Software\HAShutdownClient`

// ── Lage-niveau register-hulpfuncties ─────────────────────────────────────────

// regGetString leest een REG_SZ-waarde. Geeft defaultVal terug als de waarde
// niet bestaat of als er een fout optreedt.
func regGetString(name, defaultVal string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER, regPath, registry.QUERY_VALUE)
	if err != nil {
		return defaultVal
	}
	defer k.Close()

	val, _, err := k.GetStringValue(name)
	if err != nil {
		return defaultVal
	}
	return val
}

// regSetString schrijft een REG_SZ-waarde. Maakt de sleutel aan als die nog
// niet bestaat. Vraagt alleen SET_VALUE aan — QUERY_VALUE is niet nodig.
func regSetString(name, value string) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		regPath,
		registry.SET_VALUE,
	)
	if err != nil {
		return fmt.Errorf("register openen mislukt: %w", err)
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

// regGetInt leest een numerieke waarde die als string is opgeslagen.
func regGetInt(name string, defaultVal int) int {
	s := regGetString(name, "")
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// ── Config ───────────────────────────────────────────────────────────────────

const (
	defaultPort         = 8765
	defaultDelay        = 30
	defaultShutdownType = "shutdown"
)

// Config bevat de instelbare parameters van de client.
// Waarden worden geladen vanuit het register en kunnen worden opgeslagen
// via Save().
type Config struct {
	Port         int
	Delay        int
	ShutdownType string
}

// loadConfig laadt de instellingen vanuit het Windows-register.
// Waarden die buiten het geldige bereik vallen worden stilzwijgend vervangen
// door de standaardwaarde, zodat een handmatig aangepast of beschadigd
// register nooit leidt tot een onbruikbare configuratie.
func loadConfig() *Config {
	port := regGetInt("Port", defaultPort)
	if port < 1 || port > 65535 {
		port = defaultPort
	}

	delay := regGetInt("ShutdownDelay", defaultDelay)
	if delay < 0 || delay > 3600 {
		delay = defaultDelay
	}

	shutdownType := regGetString("ShutdownType", defaultShutdownType)
	if !isValidShutdownType(shutdownType) {
		shutdownType = defaultShutdownType
	}

	return &Config{
		Port:         port,
		Delay:        delay,
		ShutdownType: shutdownType,
	}
}

// Save slaat de huidige instellingen op in het Windows-register.
func (c *Config) Save() {
	_ = regSetString("Port", strconv.Itoa(c.Port))
	_ = regSetString("ShutdownDelay", strconv.Itoa(c.Delay))
	_ = regSetString("ShutdownType", c.ShutdownType)
}
