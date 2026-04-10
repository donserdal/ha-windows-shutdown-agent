//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/getlantern/systray"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// ── Systeemvak ────────────────────────────────────────────────────────────────

type trayMenu struct {
	mInfo     *systray.MenuItem
	mSettings *systray.MenuItem
	mAbout    *systray.MenuItem
	mLang     *systray.MenuItem
	mLangEN   *systray.MenuItem
	mLangNL   *systray.MenuItem
	mQuit     *systray.MenuItem
	hostname  string
	cfg       *Config
}

func (m *trayMenu) refreshLabels() {
	systray.SetTooltip(fmt.Sprintf(T("tray.tooltip"), m.hostname, m.cfg.Port))
	m.mInfo.SetTitle(T("tray.info"))
	m.mInfo.SetTooltip(T("tray.info.tip"))
	m.mSettings.SetTitle(T("tray.settings"))
	m.mSettings.SetTooltip(T("tray.settings.tip"))
	m.mAbout.SetTitle(T("tray.about"))
	m.mAbout.SetTooltip(T("tray.about.tip"))
	m.mLang.SetTitle(T("tray.language"))
	m.mQuit.SetTitle(T("tray.quit"))
	m.mQuit.SetTooltip(T("tray.quit.tip"))

	const check = "✓  "
	const blank = "    "
	switch GetLang() {
	case LangNL:
		m.mLangEN.SetTitle(blank + T("tray.lang.en"))
		m.mLangNL.SetTitle(check + T("tray.lang.nl"))
	default:
		m.mLangEN.SetTitle(check + T("tray.lang.en"))
		m.mLangNL.SetTitle(blank + T("tray.lang.nl"))
	}
}

func onTrayReady(cfg *Config) {
	// Sla hostnaam eenmalig op – vermijd dubbele syscall.
	hostname := mustHostname()

	systray.SetIcon(getTrayIcon())
	systray.SetTitle("HA Shutdown Client")
	systray.SetTooltip(fmt.Sprintf(T("tray.tooltip"), hostname, cfg.Port))

	menu := &trayMenu{
		hostname:  hostname,
		cfg:       cfg,
		mInfo:     systray.AddMenuItem(T("tray.info"), T("tray.info.tip")),
		mSettings: systray.AddMenuItem(T("tray.settings"), T("tray.settings.tip")),
	}
	systray.AddSeparator()
	menu.mLang = systray.AddMenuItem(T("tray.language"), "")
	menu.mLangEN = menu.mLang.AddSubMenuItem("", "English")
	menu.mLangNL = menu.mLang.AddSubMenuItem("", "Nederlands")
	systray.AddSeparator()
	menu.mAbout = systray.AddMenuItem(T("tray.about"), T("tray.about.tip"))
	systray.AddSeparator()
	menu.mQuit = systray.AddMenuItem(T("tray.quit"), T("tray.quit.tip"))
	menu.refreshLabels()

	// ── First-run firewallcheck ───────────────────────────────────────────────
	// Controleer eenmalig bij opstart of de firewallregel aanwezig is.
	// Als de regel ontbreekt én de gebruiker nog niet "niet meer vragen" heeft
	// gekozen, toon dan een informatieve prompt.
	go checkFirewallOnStartup(cfg)

	go func() {
		for {
			select {
			case <-menu.mInfo.ClickedCh:
				// showInfoDialog start intern al een goroutine; geen extra `go` nodig.
				showInfoDialog(hostname, cfg.Port, getCurrentPassword())
			case <-menu.mSettings.ClickedCh:
				showSettingsDialog(cfg)
			case <-menu.mAbout.ClickedCh:
				showAboutDialog()
			case <-menu.mLangEN.ClickedCh:
				if GetLang() != LangEN {
					SetLang(LangEN)
					menu.refreshLabels()
					go walk.MsgBox(nil, T("lang.changed.title"), T("lang.changed.body"),
						walk.MsgBoxOK|walk.MsgBoxIconInformation)
				}
			case <-menu.mLangNL.ClickedCh:
				if GetLang() != LangNL {
					SetLang(LangNL)
					menu.refreshLabels()
					go walk.MsgBox(nil, T("lang.changed.title"), T("lang.changed.body"),
						walk.MsgBoxOK|walk.MsgBoxIconInformation)
				}
			case <-menu.mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// ── First-run firewallprompt ──────────────────────────────────────────────────

// checkFirewallOnStartup controleert bij opstart of de firewallregel aanwezig
// is. Als dat niet zo is, toont het een eenmalige prompt.
// Wordt in een goroutine gestart zodat de tray al zichtbaar is.
func checkFirewallOnStartup(cfg *Config) {
	if firewallPromptDone() {
		return
	}
	if getFirewallStatus() == FirewallRulePresent {
		setFirewallPromptDone()
		return
	}

	// Korte vertraging zodat het systeemvak-icoon zichtbaar is vóór de popup.
	time.Sleep(500 * time.Millisecond)

	result := walk.MsgBox(nil,
		T("fw.prompt.title"),
		fmt.Sprintf(T("fw.prompt.body"), cfg.Port),
		walk.MsgBoxYesNoCancel|walk.MsgBoxIconQuestion,
	)

	switch result {
	case walk.DlgCmdYes:
		if err := createFirewallRule(cfg.Port); err != nil {
			log.Printf("Firewall aanmaken mislukt: %v", err)
			walk.MsgBox(nil, T("fw.err.title"),
				fmt.Sprintf(T("fw.err.body"), err),
				walk.MsgBoxOK|walk.MsgBoxIconWarning)
		} else {
			// Wacht even en controleer dan of de regel er nu is
			time.Sleep(2 * time.Second)
			refreshFirewallCache()
			if getFirewallStatus() == FirewallRulePresent {
				walk.MsgBox(nil, T("fw.ok.title"), T("fw.ok.body"),
					walk.MsgBoxOK|walk.MsgBoxIconInformation)
			}
			setFirewallPromptDone()
		}
	case walk.DlgCmdNo:
		// Gebruiker wil het zelf regelen; niet meer vragen
		setFirewallPromptDone()
	default:
		// Cancel → volgende keer opnieuw vragen
	}
}

var infoDlgOpen int32

// showInfoDialog opent het verbindingsinfo-dialoogvenster.
// Er kan slechts één exemplaar tegelijk open zijn.
func showInfoDialog(hostname string, port int, password string) {
	if !atomic.CompareAndSwapInt32(&infoDlgOpen, 0, 1) {
		return
	}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer atomic.StoreInt32(&infoDlgOpen, 0)
		if err := runInfoDialog(hostname, port, password); err != nil {
			log.Printf("Info-dialoog fout: %v", err)
		}
	}()
}

func runInfoDialog(hostname string, port int, password string) error {
	logoBmp := getLogoBitmap()

	var (
		dlg   *walk.Dialog
		okBtn *walk.PushButton
	)

	_, err := Dialog{
		AssignTo:      &dlg,
		Title:         T("info.title"),
		DefaultButton: &okBtn,
		MinSize:       Size{Width: 440, Height: 320},
		MaxSize:       Size{Width: 640, Height: 480},
		Layout:        VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{

			// ── Blauwe header met logo ─────────────────────────────────────────
			dialogHeader(
				"HA Shutdown Client",
				fmt.Sprintf("%s  ·  %s", T("info.subtitle"), hostname),
				logoBmp,
			),

			// ── Inhoud ────────────────────────────────────────────────────────
			Composite{
				Layout: VBox{
					Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 8},
					Spacing: 8,
				},
				Children: []Widget{
					Label{
						Text: fmt.Sprintf("●  %s", T("info.status.active")),
						Font: Font{Family: "Segoe UI", PointSize: 10, Bold: true},
					},
					Label{
						Text: fmt.Sprintf("%s:  %s          Port:  %d",
							T("hostname"), hostname, port),
					},
					dialogSep(),
					Label{
						Text: T("dlg.apikey.hint"),
						Font: Font{Family: "Segoe UI", PointSize: 9, Bold: true},
					},
					LineEdit{
						ReadOnly: true,
						Text:     password,
						Font:     Font{Family: "Consolas", PointSize: 9},
					},
					dialogSep(),
					privilegeStatusWidget(),
				},
			},

			// ── Footer ────────────────────────────────────────────────────────
			Composite{
				Layout: HBox{
					Margins: Margins{Left: 16, Top: 4, Right: 16, Bottom: 14},
				},
				Children: []Widget{
					PushButton{
						Text: T("info.btn.copy"),
						OnClicked: func() {
							if err := copyToClipboard(password); err != nil {
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
					HSpacer{},
					PushButton{
						AssignTo:  &okBtn,
						Text:      "OK",
						OnClicked: func() { dlg.Accept() },
					},
				},
			},
		},
	}.Run(nil)
	return err
}
