//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

var dlgOpen int32

func showSettingsDialog(cfg *Config) {
	if !atomic.CompareAndSwapInt32(&dlgOpen, 0, 1) {
		return
	}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer atomic.StoreInt32(&dlgOpen, 0)
		if err := runSettingsDialog(cfg); err != nil {
			log.Printf("Instellingen-dialoog fout: %v", err)
		}
	}()
}

func runSettingsDialog(cfg *Config) error {
	logoBmp := getLogoBitmap()

	var (
		dlg          *walk.Dialog
		saveBtn      *walk.PushButton
		cancelBtn    *walk.PushButton
		portEdit     *walk.LineEdit
		delayEdit    *walk.LineEdit
		typeCombo    *walk.ComboBox
		pwEdit       *walk.LineEdit
		autostartChk *walk.CheckBox
		fwStatusLbl  *walk.Label
	)

	typeIdx := 0
	for i, k := range typeKeys {
		if k == cfg.ShutdownType {
			typeIdx = i
			break
		}
	}
	typeNames := make([]string, len(typeKeys))
	for i, k := range typeKeys {
		typeNames[i] = T("type." + k)
	}

	_, err := Dialog{
		AssignTo:      &dlg,
		Title:         T("dlg.wintitle.settings"),
		DefaultButton: &saveBtn,
		CancelButton:  &cancelBtn,
		MinSize:       Size{Width: 460, Height: 610},
		MaxSize:       Size{Width: 700, Height: 900},
		Layout:        VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{

			// ── Blauwe header met logo ─────────────────────────────────────────
			dialogHeader(
				"HA Shutdown Client",
				fmt.Sprintf("%s  ·  %s", T("dlg.subtitle.settings"), mustHostname()),
				logoBmp,
			),

			// ── Inhoud ────────────────────────────────────────────────────────
			Composite{
				Layout: VBox{
					Margins: Margins{Left: 14, Top: 10, Right: 14, Bottom: 6},
					Spacing: 8,
				},
				Children: []Widget{

					// ── Verbinding ────────────────────────────────────────────
					GroupBox{
						Title:  T("dlg.group.connection"),
						Layout: Grid{Columns: 2, Spacing: 6},
						Children: []Widget{
							Label{Text: T("dlg.port"), MinSize: Size{Width: 140}},
							LineEdit{
								AssignTo: &portEdit,
								Text:     strconv.Itoa(cfg.Port),
							},
							Label{Text: T("dlg.delay")},
							LineEdit{
								AssignTo: &delayEdit,
								Text:     strconv.Itoa(cfg.Delay),
							},
							Label{Text: T("dlg.type")},
							ComboBox{
								AssignTo:     &typeCombo,
								Model:        typeNames,
								CurrentIndex: typeIdx,
							},
						},
					},

					// ── Beveiliging ───────────────────────────────────────────
					GroupBox{
						Title:  T("dlg.group.security"),
						Layout: Grid{Columns: 2, Spacing: 6},
						Children: []Widget{
							Label{Text: T("dlg.apikey"), MinSize: Size{Width: 140}},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 4},
								Children: []Widget{
									LineEdit{
										AssignTo: &pwEdit,
										ReadOnly: true,
										Text:     getCurrentPassword(),
										Font:     Font{Family: "Consolas", PointSize: 9},
									},
									PushButton{
										Text:        "↺",
										MaxSize:     Size{Width: 32},
										ToolTipText: T("dlg.btn.refresh"),
										OnClicked: func() {
											pw := refreshActivePassword()
											pwEdit.SetText(pw)
											walk.MsgBox(dlg, T("pw.reset.title"),
												fmt.Sprintf(T("pw.reset.body"), pw),
												walk.MsgBoxOK|walk.MsgBoxIconInformation)
										},
									},
								},
							},
							Label{},
							PushButton{
								Text: T("dlg.btn.copy"),
								OnClicked: func() {
									if err := copyToClipboard(getCurrentPassword()); err != nil {
										walk.MsgBox(dlg, T("dlg.err.title"),
											fmt.Sprintf("Copy failed: %v", err),
											walk.MsgBoxOK|walk.MsgBoxIconWarning)
										return
									}
									walk.MsgBox(dlg, "HA Shutdown Client",
										T("info.copied"),
										walk.MsgBoxOK|walk.MsgBoxIconInformation)
								},
							},
						},
					},

					// ── Systeem ───────────────────────────────────────────────
					GroupBox{
						Title:  T("dlg.group.system"),
						Layout: VBox{Margins: Margins{Left: 8, Top: 4, Right: 8, Bottom: 8}, Spacing: 6},
						Children: []Widget{
							CheckBox{
								AssignTo: &autostartChk,
								Text:     T("dlg.autostart"),
								Checked:  isAutoStartEnabled(),
							},
							dialogSep(),
							privilegeStatusWidget(),
						},
					},

					// ── Firewall ──────────────────────────────────────────────
					GroupBox{
						Title:  T("dlg.group.firewall"),
						Layout: Grid{Columns: 2, Spacing: 6, Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 8}},
						Children: []Widget{
							Label{Text: T("dlg.fw.status"), MinSize: Size{Width: 140}},
							Label{
								AssignTo:  &fwStatusLbl,
								Text:      func() string { t, _ := firewallStatusText(); return t }(),
								TextColor: func() walk.Color { _, c := firewallStatusText(); return c }(),
								Font:      Font{Family: "Segoe UI", PointSize: 9},
							},
							Label{},
							Composite{
								Layout: HBox{MarginsZero: true, Spacing: 6},
								Children: []Widget{
									PushButton{
										Text:    T("dlg.fw.btn.create"),
										MinSize: Size{Width: 130},
										OnClicked: func() {
											port := cfg.Port
											if p, err := strconv.Atoi(portEdit.Text()); err == nil && p > 0 && p < 65536 {
												port = p
											}
											if err := createFirewallRule(port); err != nil {
												walk.MsgBox(dlg, T("fw.err.title"),
													fmt.Sprintf(T("fw.err.body"), err),
													walk.MsgBoxOK|walk.MsgBoxIconWarning)
												return
											}
											// Wacht buiten de UI-thread tot netsh klaar is,
											// en update het label daarna via Synchronize zodat
											// walk de UI-update op de correcte thread uitvoert.
											go func() {
												time.Sleep(2 * time.Second)
												refreshFirewallCache()
												txt, clr := firewallStatusText()
												// Synchronize marshalt de UI-update terug naar de
												// message-loop thread; zonder dit riskeert walk een crash.
												fwStatusLbl.Synchronize(func() {
													fwStatusLbl.SetText(txt)
													fwStatusLbl.SetTextColor(clr)
												})
											}()
											walk.MsgBox(dlg, T("fw.prompt.title"),
												T("fw.creating.body"),
												walk.MsgBoxOK|walk.MsgBoxIconInformation)
										},
									},
									PushButton{
										Text:    T("dlg.fw.btn.refresh"),
										MinSize: Size{Width: 80},
										OnClicked: func() {
											refreshFirewallCache()
											txt, clr := firewallStatusText()
											fwStatusLbl.SetText(txt)
											fwStatusLbl.SetTextColor(clr)
										},
									},
								},
							},
						},
					},
				},
			},

			// ── Footer ────────────────────────────────────────────────────────
			dialogSep(),
			Composite{
				Layout: HBox{
					Margins: Margins{Left: 14, Top: 8, Right: 14, Bottom: 12},
				},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo:  &cancelBtn,
						Text:      T("dlg.btn.cancel"),
						OnClicked: func() { dlg.Cancel() },
					},
					PushButton{
						AssignTo: &saveBtn,
						Text:     T("dlg.btn.save"),
						OnClicked: func() {
							if applySettings(dlg, cfg, portEdit, delayEdit, typeCombo, autostartChk) {
								dlg.Accept()
							}
						},
					},
				},
			},
		},
	}.Run(nil)
	return err
}

func applySettings(
	owner *walk.Dialog,
	cfg *Config,
	portEdit, delayEdit *walk.LineEdit,
	typeCombo *walk.ComboBox,
	autostartChk *walk.CheckBox,
) bool {
	port, err := strconv.Atoi(portEdit.Text())
	if err != nil || port < 1 || port > 65535 {
		walk.MsgBox(owner, T("dlg.err.title"), T("dlg.err.port"),
			walk.MsgBoxOK|walk.MsgBoxIconWarning)
		portEdit.SetFocus()
		return false
	}

	delay, err := strconv.Atoi(delayEdit.Text())
	if err != nil || delay < 0 || delay > 3600 {
		walk.MsgBox(owner, T("dlg.err.title"), T("dlg.err.delay"),
			walk.MsgBoxOK|walk.MsgBoxIconWarning)
		delayEdit.SetFocus()
		return false
	}

	idx := typeCombo.CurrentIndex()
	if idx < 0 || idx >= len(typeKeys) {
		walk.MsgBox(owner, T("dlg.err.title"), T("dlg.err.type"),
			walk.MsgBoxOK|walk.MsgBoxIconWarning)
		return false
	}

	wantAutostart := autostartChk.Checked()
	if wantAutostart != isAutoStartEnabled() {
		if aErr := setAutoStart(wantAutostart); aErr != nil {
			walk.MsgBox(owner, T("dlg.err.title"),
				fmt.Sprintf("%s\r\n%v", T("dlg.err.autostart"), aErr),
				walk.MsgBoxOK|walk.MsgBoxIconWarning)
		}
	}

	changed := port != cfg.Port || delay != cfg.Delay || typeKeys[idx] != cfg.ShutdownType
	cfg.Port = port
	cfg.Delay = delay
	cfg.ShutdownType = typeKeys[idx]
	cfg.Save()

	if changed {
		walk.MsgBox(owner, T("dlg.saved.title"),
			fmt.Sprintf(T("dlg.saved.body"), port, delay, T("type."+typeKeys[idx])),
			walk.MsgBoxOK|walk.MsgBoxIconInformation)
	}
	return true
}


