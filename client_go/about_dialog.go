//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime"
	"sync/atomic"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

var aboutDlgOpen int32

// showAboutDialog opent het About-venster. Er kan slechts één exemplaar tegelijk
// open zijn; een tweede aanroep keert onmiddellijk terug.
func showAboutDialog() {
	if !atomic.CompareAndSwapInt32(&aboutDlgOpen, 0, 1) {
		return
	}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer atomic.StoreInt32(&aboutDlgOpen, 0)
		if err := runAboutDialog(); err != nil {
			log.Printf("About-dialoog fout: %v", err)
		}
	}()
}

func runAboutDialog() error {
	logoBmp := getLogoBitmap()

	// Kies het logo-widget eenmalig vóór de struct-literal zodat er geen
	// anonieme functies in de widget-boom zitten.
	var bigLogo Widget
	if logoBmp != nil {
		bigLogo = ImageView{
			Image:   logoBmp,
			MinSize: Size{Width: 80, Height: 80},
			MaxSize: Size{Width: 80, Height: 80},
			Mode:    ImageViewModeZoom,
		}
	} else {
		bigLogo = HSpacer{MinSize: Size{Width: 80}}
	}

	var (
		dlg   *walk.Dialog
		okBtn *walk.PushButton
	)

	_, err := Dialog{
		AssignTo:      &dlg,
		Title:         T("about.wintitle"),
		DefaultButton: &okBtn,
		MinSize:       Size{Width: 420, Height: 380},
		MaxSize:       Size{Width: 520, Height: 480},
		Layout:        VBox{MarginsZero: true, SpacingZero: true},
		Children: []Widget{

			// ── Blauwe header ─────────────────────────────────────────────────
			dialogHeader("HA Shutdown Client", T("about.subtitle"), logoBmp),

			// ── Logo (gecentreerd, groter) ────────────────────────────────────
			Composite{
				Layout: HBox{Margins: Margins{Top: 20, Bottom: 4}},
				Children: []Widget{
					HSpacer{},
					bigLogo,
					HSpacer{},
				},
			},

			// ── Tekst ─────────────────────────────────────────────────────────
			Composite{
				Layout: VBox{
					Margins: Margins{Left: 24, Top: 8, Right: 24, Bottom: 8},
					Spacing: 4,
				},
				Children: []Widget{
					Label{
						Text:      "HA Shutdown Client",
						Font:      Font{Family: "Segoe UI", PointSize: 13, Bold: true},
						Alignment: AlignHCenterVCenter,
					},
					Label{
						Text:      fmt.Sprintf("%s %s", T("version"), version),
						Font:      Font{Family: "Segoe UI", PointSize: 9},
						Alignment: AlignHCenterVCenter,
					},
					VSpacer{MinSize: Size{Height: 6}},
					Label{
						Text:      T("about.description"),
						Font:      Font{Family: "Segoe UI", PointSize: 9},
						Alignment: AlignHCenterVCenter,
					},
					VSpacer{MinSize: Size{Height: 6}},
					dialogSep(),
					VSpacer{MinSize: Size{Height: 6}},
					Label{
						Text:      T("about.copyright"),
						Font:      Font{Family: "Segoe UI", PointSize: 8},
						Alignment: AlignHCenterVCenter,
						TextColor: walk.RGB(100, 100, 100),
					},
					Label{
						Text:      T("about.license"),
						Font:      Font{Family: "Segoe UI", PointSize: 8},
						Alignment: AlignHCenterVCenter,
						TextColor: walk.RGB(100, 100, 100),
					},
				},
			},

			// ── Spacer + footer ───────────────────────────────────────────────
			VSpacer{},
			dialogSep(),
			Composite{
				Layout: HBox{
					Margins: Margins{Left: 16, Top: 8, Right: 16, Bottom: 12},
				},
				Children: []Widget{
					HSpacer{},
					PushButton{
						AssignTo:  &okBtn,
						Text:      "OK",
						MinSize:   Size{Width: 80},
						OnClicked: func() { dlg.Accept() },
					},
				},
			},
		},
	}.Run(nil)
	return err
}
