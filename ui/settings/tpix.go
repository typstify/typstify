package settings

import (
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	gvwidget "github.com/oligo/gioview/widget"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/widgets/icons"
)

const (
	tpixUrl = "https://tpix.typstify.com"
)

var userIcon = icons.NewSvgIcon(icons.User)

type TpixSettingsView struct {
	srv             *service.ServiceFacade
	apiKeyInput     gvwidget.TextField
	tpixWebsiteLink widget.Clickable
	saveBtn         widget.Clickable
	profileUpdating atomic.Bool
	once            sync.Once
	lastErr         error
}

func (t *TpixSettingsView) Title() string {
	return i18n.Translate("TPIX")
}

func (t *TpixSettingsView) update(gtx C) {
	t.once.Do(func() {
		// update each time the view is opened.
		t.lastErr = t.updateProfile()
	})

	if t.saveBtn.Clicked(gtx) {
		t.lastErr = nil
		setting := t.srv.Settings().Tpix()
		apiKey := strings.TrimSpace(t.apiKeyInput.Text())
		if apiKey == "" {
			setting.Username = ""
			setting.Email = ""
		}
		setting.ApiKey = apiKey
		err := setting.Save()
		if err != nil {
			t.lastErr = err
			return
		}

		// also update user profile
		err = t.updateProfile()
		if err != nil {
			t.lastErr = err
			return
		}
	}

	if t.tpixWebsiteLink.Clicked(gtx) {
		if err := giohyperlink.Open(tpixUrl); err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
	}

}

func (t *TpixSettingsView) updateProfile() error {
	if !t.profileUpdating.CompareAndSwap(false, true) {
		return nil
	}
	defer t.profileUpdating.Store(false)

	if !t.srv.Authenticated() {
		return nil
	}

	profile, err := t.srv.PkgService().TpixClient().GetUserProfile()
	if err != nil {
		log.Printf("update profile error: %s", err)
		return err
	}

	setting := t.srv.Settings().Tpix()
	setting.Username = profile.Username
	setting.Email = profile.Email
	setting.Save()

	return nil
}

func (t *TpixSettingsView) Layout(gtx C, th *theme.Theme) D {
	t.update(gtx)

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			if t.lastErr != nil {
				return misc.LayoutErrorLabel(gtx, th, t.lastErr)

			} else {
				return layout.Dimensions{}
			}
		}),

		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, i18n.Translate("TPIX is a free cloud search that you can use to find & manage personal or team packages, and even bibliographies."))
			label.LineHeightScale = 1.5

			return label.Layout(gtx)
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, i18n.Translate("Paste your API Key issued from TPIX server to start using TPIX service."))
			label.LineHeightScale = 1.5

			return label.Layout(gtx)
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx C) D {
			return layout.Flex{
				Axis: layout.Horizontal,
			}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					t.apiKeyInput.Alignment = text.Start
					t.apiKeyInput.SingleLine = true
					return t.apiKeyInput.Layout(gtx, th, "Paste API key here...")
				}),

				layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),

				layout.Rigid(func(gtx C) D {
					return material.Button(th.Theme, &t.saveBtn, i18n.Translate("Save")).Layout(gtx)
				}),
			)

		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(32)}.Layout),

		layout.Rigid(func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return material.Label(th.Theme, th.TextSize, i18n.Translate("To learn more about TPIX, go to ")).Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					return material.Clickable(gtx, &t.tpixWebsiteLink, func(gtx C) D {
						label := material.Label(th.Theme, th.TextSize, tpixUrl)
						label.Color = th.ContrastBg
						return label.Layout(gtx)
					})
				}),
			)
		}),
	)

}
