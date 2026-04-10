//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"sync"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// ── Gedeelde UI-hulpfuncties ──────────────────────────────────────────────────

// dialogSep tekent een dunne horizontale scheidingslijn (1 px grijs).
func dialogSep() Widget {
	return Composite{
		Background: SolidColorBrush{Color: walk.RGB(220, 220, 220)},
		MinSize:    Size{Height: 1},
		MaxSize:    Size{Height: 1},
		Layout:     HBox{MarginsZero: true, SpacingZero: true},
	}
}

// dialogHeader bouwt de blauwe titelbalk die boven in elk dialoogvenster staat.
// Als logoBmp niet nil is, wordt het logo links van de titeltekst getoond.
func dialogHeader(title, subtitle string, logoBmp *walk.Bitmap) Widget {
	var logoWidget Widget
	if logoBmp != nil {
		logoWidget = ImageView{
			Image:   logoBmp,
			MinSize: Size{Width: 44, Height: 44},
			MaxSize: Size{Width: 44, Height: 44},
			Mode:    ImageViewModeZoom,
		}
	} else {
		logoWidget = HSpacer{MinSize: Size{Width: 4}}
	}

	return Composite{
		Background: SolidColorBrush{Color: walk.RGB(27, 79, 138)},
		MinSize:    Size{Height: 72},
		MaxSize:    Size{Height: 72},
		Layout: HBox{
			Margins: Margins{Left: 14, Top: 12, Right: 18, Bottom: 12},
			Spacing: 12,
		},
		Children: []Widget{
			logoWidget,
			Composite{
				Layout: VBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					VSpacer{},
					Label{
						Text:      title,
						Font:      Font{Family: "Segoe UI", PointSize: 12, Bold: true},
						TextColor: walk.RGB(255, 255, 255),
					},
					Label{
						Text:      subtitle,
						Font:      Font{Family: "Segoe UI", PointSize: 9},
						TextColor: walk.RGB(175, 205, 230),
					},
					VSpacer{},
				},
			},
		},
	}
}

// ── Status-widgets ────────────────────────────────────────────────────────────

// privilegeStatusWidget geeft een Label-widget terug met de huidige
// afsluitprivilege-status, inclusief kleurcodering.
func privilegeStatusWidget() Widget {
	ok, err := getPrivilegeStatus()
	var text string
	var color walk.Color
	switch {
	case err != nil:
		text = fmt.Sprintf("%s  (%v)", T("priv.unknown"), err)
		color = walk.RGB(120, 120, 120)
	case ok:
		text = T("priv.ok")
		color = walk.RGB(0, 140, 0)
	default:
		text = T("priv.denied")
		color = walk.RGB(200, 30, 30)
	}
	return Label{
		Text:      text,
		TextColor: color,
		Font:      Font{Family: "Segoe UI", PointSize: 9},
	}
}

// firewallStatusText geeft de tekst en kleur voor de huidige firewallstatus.
func firewallStatusText() (text string, color walk.Color) {
	switch getFirewallStatus() {
	case FirewallRulePresent:
		return T("fw.status.ok"), walk.RGB(0, 140, 0)
	case FirewallRuleMissing:
		return T("fw.status.missing"), walk.RGB(200, 30, 30)
	default:
		return T("fw.status.unknown"), walk.RGB(120, 120, 120)
	}
}

// ── Logo-bitmap cache ─────────────────────────────────────────────────────────
//
// De bitmap wordt één keer gedekodeerd en gecachet. Alle dialoogvensters
// delen dezelfde instantie; zo wordt bij elk openen niet opnieuw een PNG
// gedekodeerd en een bitmap gealloceerd.

var (
	logoBmpOnce sync.Once
	logoBmpVal  *walk.Bitmap
)

// getLogoBitmap decodeert het ingebedde PNG-logo en geeft een *walk.Bitmap terug.
// Bij een fout wordt nil teruggegeven; de UI valt dan terug op een lege spacer.
// Het resultaat wordt gecachet via sync.Once.
func getLogoBitmap() *walk.Bitmap {
	logoBmpOnce.Do(func() {
		img, err := png.Decode(bytes.NewReader(logoPNG))
		if err != nil {
			log.Printf("logo decode fout: %v", err)
			return
		}
		bmp, err := walk.NewBitmapFromImage(img)
		if err != nil {
			log.Printf("logo bitmap fout: %v", err)
			return
		}
		logoBmpVal = bmp
	})
	return logoBmpVal
}
