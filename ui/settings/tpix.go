package settings

import (
	"fmt"
	"log"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/widgets/icons"
)

const (
	subscriptionUrl = "https://typstify.com/tpix"
	tpixUrl         = "https://tpix.typstify.com"
)

var userIcon = icons.NewSvgIcon(icons.User)

type TpixSettingsView struct {
	srv              *service.TpixSessionService
	loginBtn         widget.Clickable
	logoutBtn        widget.Clickable
	tpixWebsiteLink  widget.Clickable
	subscriptionLink widget.Clickable

	lastErr error
}

func (t *TpixSettingsView) Title() string {
	return i18n.Translate("TPIX")
}

func (t *TpixSettingsView) update(gtx C) {
	if t.loginBtn.Clicked(gtx) {
		go t.srv.Login()
	}

	if t.logoutBtn.Clicked(gtx) {
		t.srv.Logout()
	}

	if t.tpixWebsiteLink.Clicked(gtx) {
		if err := giohyperlink.Open(tpixUrl); err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
	}

	if t.subscriptionLink.Clicked(gtx) {
		if err := giohyperlink.Open(subscriptionUrl); err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
	}
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
			if !t.srv.Authenticated() {
				return layout.Dimensions{}
			}

			session := t.srv.Session()
			if session == nil {
				return material.Label(th.Theme, th.TextSize, i18n.Translate("Loading...")).Layout(gtx)
			}

			return settingItem{}.Layout(gtx, th, i18n.Translate("You have an active TPIX session"),
				"",
				func(gtx C) D {
					return layout.Flex{
						Axis:      layout.Vertical,
						Alignment: layout.Start,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							return layout.Flex{
								Alignment: layout.Middle,
								Gap:       gtx.Dp(unit.Dp(4)),
							}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return userIcon.Layout(gtx, th.ContrastBg, th.TextSize)
								}),
								layout.Rigid(func(gtx C) D {
									return material.Label(th.Theme, th.TextSize, fmt.Sprintf("%s <%s>", session.Username, session.Email)).Layout(gtx)
								}),
							)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

						layout.Rigid(func(gtx C) D {
							return material.Label(th.Theme, th.TextSize, fmt.Sprintf("Subscribed: %t", session.Subscribed)).Layout(gtx)
						}),

						layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

						layout.Rigid(func(gtx C) D {
							return material.Label(th.Theme, th.TextSize, fmt.Sprintf("Logged in at: %s", session.LastLoginAt)).Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx C) D {
							btn := material.Button(th.Theme, &t.logoutBtn, i18n.Translate("Logout TPIX"))
							return btn.Layout(gtx)
						}),
					)
				})
		}),

		layout.Rigid(func(gtx C) D {
			if t.srv.Authenticated() {
				return layout.Dimensions{}
			}

			return layout.Flex{
				Axis:      layout.Vertical,
				Alignment: layout.Start,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize, i18n.Translate("Login TPIX to access all the features of Typstify, including package management, Zotero sync, MCP tools for AI assistant etc. Some features may need a subscription."))
					label.LineHeightScale = 1.5
					return label.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx C) D {
					btn := material.Button(th.Theme, &t.loginBtn, i18n.Translate("Login TPIX"))
					return btn.Layout(gtx)
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

		layout.Rigid(func(gtx C) D {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return material.Label(th.Theme, th.TextSize, i18n.Translate("To get a subscription of TPIX, click ")).Layout(gtx)
				}),
				layout.Rigid(func(gtx C) D {
					return material.Clickable(gtx, &t.subscriptionLink, func(gtx C) D {
						label := material.Label(th.Theme, th.TextSize, subscriptionUrl)
						label.Color = th.ContrastBg
						return label.Layout(gtx)
					})
				}),
			)

		}),
	)

}
