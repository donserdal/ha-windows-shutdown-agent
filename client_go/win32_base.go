//go:build windows

package main

import "golang.org/x/sys/windows"

// user32dll is de gedeelde handle naar user32.dll.
// Gebruikt door clipboard.go (OpenClipboard, SetClipboardData, etc.).
// windows.NewLazySystemDLL zoekt uitsluitend in System32 — geen CWD-risico.
var user32dll = windows.NewLazySystemDLL("user32.dll")
