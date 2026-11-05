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
	clearBtn        widget.Clickable
	profileUpdating atomic.Bool
	once            sync.Once

	// pendingProfile is the profile fetched by the background task.
	pendingProfile atomic.Pointer[tpixProfile]

	mu      sync.Mutex
	lastErr error
}

// tpixProfile holds the part of the user profile that is stored in the settings.
type tpixProfile struct {
	username string
	email    string
}

func (t *TpixSettingsView) Title() string {
	return i18n.Translate("TPIX")
}

func (t *TpixSettingsView) update(gtx C) {
	t.once.Do(t.refreshProfile)

	// Apply a profile fetched in the background. A response arriving after the
	// user cleared the key must not restore the old account info.
	if profile := t.pendingProfile.Swap(nil); profile != nil && t.srv.Authenticated() {
		setting := t.srv.Settings().Tpix()
		setting.Username = profile.username
		setting.Email = profile.email
		if err := setting.Save(); err != nil {
			t.setErr(err)
		}
	}

	if t.saveBtn.Clicked(gtx) {
		t.setErr(nil)
		setting := t.srv.Settings().Tpix()
		apiKey := strings.TrimSpace(t.apiKeyInput.Text())
		if apiKey == "" {
			setting.Username = ""
			setting.Email = ""
		}
		setting.ApiKey = apiKey
		err := setting.Save()
		if err != nil {
			t.setErr(err)
			return
		}

		// The key is not shown once it is stored, so do not keep it in the input.
		t.apiKeyInput.SetText("")

		// Also update the user profile. This hits the network, so it is done in
		// the background and applied once it is available.
		t.refreshProfile()
	}

	if t.clearBtn.Clicked(gtx) {
		t.setErr(nil)
		setting := t.srv.Settings().Tpix()
		setting.ApiKey = ""
		setting.Username = ""
		setting.Email = ""
		if err := setting.Save(); err != nil {
			t.setErr(err)
			return
		}

		// Show the input again so a new key can be pasted.
		t.apiKeyInput.SetText("")
	}

	if t.tpixWebsiteLink.Clicked(gtx) {
		if err := giohyperlink.Open(tpixUrl); err != nil {
			log.Printf("error: opening hyperlink: %v", err)
		}
	}

}

// apiKeyPrefix returns the recognizable part of an API key so the user can tell
// which key is configured without exposing it.
func apiKeyPrefix(key string) string {
	const prefixLen = 8

	if len(key) <= prefixLen {
		return strings.Repeat("*", len(key))
	}

	return key[:prefixLen] + "\u2026"
}

// refreshProfile fetches the TPIX user profile in the background. Requesting it
// on the UI goroutine would block the window until the server responds, e.g.
// while the settings page is opening, which makes navigation feel frozen.
func (t *TpixSettingsView) refreshProfile() {
	if !t.srv.Authenticated() {
		return
	}

	if !t.profileUpdating.CompareAndSwap(false, true) {
		return
	}

	srv := t.srv
	go func() {
		defer t.profileUpdating.Store(false)

		profile, err := srv.PkgService().TpixClient().GetUserProfile()
		if err != nil {
			log.Printf("update profile error: %s", err)
			t.setErr(err)
		} else {
			t.setErr(nil)
			t.pendingProfile.Store(&tpixProfile{
				username: profile.Username,
				email:    profile.Email,
			})
		}

		// Render the updated profile or the error.
		srv.RefreshWindow()
	}()
}

func (t *TpixSettingsView) setErr(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastErr = err
}

func (t *TpixSettingsView) err() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	return t.lastErr
}

func (t *TpixSettingsView) Layout(gtx C, th *theme.Theme) D {
	t.update(gtx)

	// Only show the API key input when there is no key yet, so a stored key is
	// displayed as a recognisable prefix instead of an empty input box.
	if t.srv.Authenticated() {
		return t.layoutAccount(gtx, th)
	}

	return t.layoutAPIKeyInput(gtx, th)
}

func (t *TpixSettingsView) layoutAccount(gtx C, th *theme.Theme) D {
	setting := t.srv.Settings().Tpix()

	return t.layoutPage(gtx, th,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return layout.Inset{Right: unit.Dp(10)}.Layout(gtx, func(gtx C) D {
						return userIcon.Layout(gtx, th.ContrastBg, th.TextSize*1.4)
					})
				}),
				layout.Rigid(func(gtx C) D {
					return layout.Flex{
						Axis: layout.Vertical,
					}.Layout(gtx,
						layout.Rigid(func(gtx C) D {
							if setting.Username == "" {
								return layout.Dimensions{}
							}

							return material.Label(th.Theme, th.TextSize, setting.Username).Layout(gtx)
						}),
						layout.Rigid(func(gtx C) D {
							if setting.Email == "" {
								return layout.Dimensions{}
							}

							label := material.Label(th.Theme, th.TextSize*0.9, setting.Email)
							label.Color = misc.WithAlpha(th.Fg, 0xb6)
							return label.Layout(gtx)
						}),
					)
				}),
			)
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		layout.Rigid(func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize, i18n.Translate("API Key:"))
					label.Color = misc.WithAlpha(th.Fg, 0xb6)
					return label.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx C) D {
					return material.Label(th.Theme, th.TextSize, apiKeyPrefix(setting.ApiKey)).Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
				layout.Rigid(func(gtx C) D {
					btn := material.Button(th.Theme, &t.clearBtn, i18n.Translate("Clear"))
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		}),
	)
}

func (t *TpixSettingsView) layoutAPIKeyInput(gtx C, th *theme.Theme) D {
	return t.layoutPage(gtx, th,
		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, i18n.Translate("Paste your API Key issued from TPIX server to start using TPIX service."))
			label.LineHeightScale = 1.5

			return label.Layout(gtx)
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
			}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					t.apiKeyInput.Alignment = text.Start
					t.apiKeyInput.SingleLine = true
					return t.apiKeyInput.Layout(gtx, th, "Paste API key here...")
				}),

				layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),

				layout.Rigid(func(gtx C) D {
					btn := material.Button(th.Theme, &t.saveBtn, i18n.Translate("Save"))
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		}),
	)
}

// layoutPage renders the chrome shared by both states of the page.
func (t *TpixSettingsView) layoutPage(gtx C, th *theme.Theme, body ...layout.FlexChild) D {
	children := make([]layout.FlexChild, 0, len(body)+5)
	children = append(children,
		layout.Rigid(func(gtx C) D {
			if err := t.err(); err != nil {
				return misc.LayoutErrorLabel(gtx, th, err)
			}

			return layout.Dimensions{}
		}),

		layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, i18n.Translate("TPIX is a cloud service that you can use to find & manage personal or team packages, and even bibliographies."))
			label.LineHeightScale = 1.5

			return label.Layout(gtx)
		}),

		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
	)

	children = append(children, body...)

	children = append(children,
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

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx, children...)
}
