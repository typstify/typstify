package settings

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/page"
	"github.com/oligo/gioview/tabview"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"

	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
)

var (
	SettingViewID = view.NewViewID("Settings")
)

type (
	C = layout.Context
	D = layout.Dimensions
)
type SettingsView struct {
	*view.BaseView
	page.PageStyle
	srv         *service.ServiceFacade
	subSettings []SubSettingView
	tabView     *tabview.TabView
}

func (sv *SettingsView) ID() view.ViewID {
	return SettingViewID
}

func (sv *SettingsView) Title() string {
	return i18n.Translate("Settings")
}

func (sv *SettingsView) Layout(gtx layout.Context, th *theme.Theme) layout.Dimensions {
	sv.PageStyle.Padding = unit.Dp(30)
	sv.MaxWidth = unit.Dp(960)

	return layout.Inset{
		Top: unit.Dp(60),
	}.Layout(gtx, func(gtx C) D {
		return sv.PageStyle.Layout(gtx, th, func(gtx C) D {
			return sv.tabView.Layout(gtx, th)
		})
	})

}

func NewSettingsView(srv *service.ServiceFacade) *SettingsView {
	tabItems := make([]*tabview.TabItem, 0)

	general := srv.Settings().General()
	editor := srv.Settings().Editor()

	subSettings := []SubSettingView{
		&GeneralView{setting: general},
		&EditorView{setting: editor},
		&TypstSettingsView{setting: srv.Settings().Typst()},
		&TpixSettingsView{setting: srv.Settings().Tpix()},
		&AgentView{setting: srv.Settings().AcpAgent()},
		&HelpView{updateCheck: &UpdateCheck{srv: srv}},
	}

	inset := layout.Inset{
		Left:   unit.Dp(16),
		Right:  unit.Dp(48),
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
	}

	for _, subSetting := range subSettings {
		subSetting := subSetting
		tabItems = append(tabItems,
			tabview.NewTabItem(inset, func(gtx C, th *theme.Theme) D {
				label := material.Label(th.Theme, th.TextSize, i18n.Translate(subSetting.Title()))
				label.Font.Weight = font.Medium
				return label.Layout(gtx)
			},

				func(gtx C, th *theme.Theme) D {
					return subSetting.Layout(gtx, th)
				}),
		)
	}

	return &SettingsView{
		BaseView:    &view.BaseView{},
		srv:         srv,
		subSettings: subSettings,
		tabView:     tabview.NewTabView(layout.Vertical, tabItems...),
	}
}
