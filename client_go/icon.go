//go:build windows

package main

import _ "embed"

// trayIconICO bevat het ICO-bestand dat in het systeemvak én als app-icoon wordt getoond.
// Ingebakken via go:embed; geen losse bestanden nodig naast de .exe.
//
//go:embed homeassistant_windows_shutdown.ico
var trayIconICO []byte

// logoPNG bevat het PNG-logo dat in dialoogvensters wordt getoond.
//
//go:embed logo.png
var logoPNG []byte

// getTrayIcon geeft de bytes van het systeemvak-pictogram terug.
func getTrayIcon() []byte { return trayIconICO }
