package view

import (
	"context"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/agent"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/widgets"
)

type SessionConfigStyle struct {
	Session *agent.ACPSession
	popups  map[string]*configCategoryPopup
}

type configCategoryPopup struct {
	btn   widget.Clickable
	popup widgets.Popup
}

func (s *SessionConfigStyle) Layout(gtx C, th *theme.Theme) D {
	configs := s.Session.ConfigOptions()
	if len(configs) == 0 {
		return D{}
	}

	// Group by category.
	categories := make(map[string][]acp.SessionConfigOption)
	var order []string
	for _, c := range configs {
		cat := ""
		if c.Select != nil && c.Select.Name != "" {
			cat = c.Select.Name
		} else if c.Boolean != nil && c.Boolean.Name != "" {
			cat = c.Boolean.Name
		}
		if _, exists := categories[cat]; !exists {
			order = append(order, cat)
		}
		categories[cat] = append(categories[cat], c)
	}

	// Ensure popup state exists.
	if s.popups == nil {
		s.popups = make(map[string]*configCategoryPopup)
	}
	for _, cat := range order {
		if s.popups[cat] == nil {
			s.popups[cat] = &configCategoryPopup{}
		}
	}

	children := make([]layout.FlexChild, 0, len(order)*2)
	for i, cat := range order {
		cat := cat
		st := s.popups[cat]
		catConfigs := categories[cat]

		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx C) D {
			st.popup.Direction = layout.N
			st.popup.Width = unit.Dp(250)
			return st.popup.Layout(gtx, th,
				func(gtx C) D {
					if st.btn.Clicked(gtx) {
						st.popup.SetOpen()
					}
					return material.Clickable(gtx, &st.btn, func(gtx C) D {
						return layout.Inset{
							Top: unit.Dp(2), Bottom: unit.Dp(2),
							Left: unit.Dp(4), Right: unit.Dp(4),
						}.Layout(gtx, func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize*0.8, categoryLabel(cat, catConfigs))
							label.Color = misc.WithAlpha(th.Fg, 0xb0)
							return label.Layout(gtx)
						})
					})
				},
				s.buildPopupItems(catConfigs)...,
			)
		}))
	}

	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (s *SessionConfigStyle) buildPopupItems(configs []acp.SessionConfigOption) []widgets.PopupWidget {
	var items []widgets.PopupWidget
	for _, c := range configs {
		if c.Select != nil {
			options := c.Select.Options
			if options.Ungrouped != nil {
				for _, opt := range *options.Ungrouped {
					opt := opt
					items = append(items, &configSelectItem{
						opt:      opt,
						configId: c.Select.Id,
						session:  s.Session,
					})
				}
			}
			if options.Grouped != nil {
				for _, group := range *options.Grouped {
					for _, opt := range group.Options {
						opt := opt
						items = append(items, &configSelectItem{
							opt:      opt,
							configId: c.Select.Id,
							session:  s.Session,
						})
					}
				}
			}
		}
		if c.Boolean != nil {
			items = append(items, &configBooleanItem{
				opt:     *c.Boolean,
				session: s.Session,
			})
		}
	}
	return items
}

// configSelectItem implements widgets.PopupWidget for select options.
type configSelectItem struct {
	opt      acp.SessionConfigSelectOption
	configId acp.SessionConfigId
	session  *agent.ACPSession
}

func (c *configSelectItem) OnClicked() {
	go func() {
		c.session.UpdateConfig(context.Background(), c.configId, c.opt.Value)
	}()
}

func (c *configSelectItem) Layout(gtx C, th *theme.Theme) D {
	return layout.Inset{
		Top: unit.Dp(6), Bottom: unit.Dp(6),
		Left: unit.Dp(12), Right: unit.Dp(12),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				label := material.Label(th.Theme, th.TextSize, c.opt.Name)
				label.Color = th.Fg
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				if c.opt.Description != nil && *c.opt.Description != "" {
					desc := material.Label(th.Theme, th.TextSize*0.75, *c.opt.Description)
					desc.Color = misc.WithAlpha(th.Fg, 0xb0)
					return desc.Layout(gtx)
				}
				return D{}
			}),
		)
	})
}

// configBooleanItem implements widgets.PopupWidget for boolean options.
type configBooleanItem struct {
	opt     acp.SessionConfigOptionBoolean
	session *agent.ACPSession
}

func (c *configBooleanItem) OnClicked() {
	go func() {
		c.session.UpdateConfig(context.Background(), c.opt.Id, !c.opt.CurrentValue)
	}()
}

func (c *configBooleanItem) Layout(gtx C, th *theme.Theme) D {
	val := i18n.Translate("Off")
	if c.opt.CurrentValue {
		val = i18n.Translate("On")
	}
	return layout.Inset{
		Top: unit.Dp(6), Bottom: unit.Dp(6),
		Left: unit.Dp(12), Right: unit.Dp(12),
	}.Layout(gtx, func(gtx C) D {
		label := material.Label(th.Theme, th.TextSize, c.opt.Name+": "+val)
		label.Color = th.Fg
		return label.Layout(gtx)
	})
}

func categoryLabel(cat string, configs []acp.SessionConfigOption) string {
	name := cat
	if name == "" {
		name = i18n.Translate("Options")
	}
	if len(configs) == 1 && configs[0].Select != nil {
		sel := configs[0].Select
		currentName := string(sel.CurrentValue)
		if sel.Options.Ungrouped != nil {
			for _, o := range *sel.Options.Ungrouped {
				if o.Value == sel.CurrentValue {
					currentName = o.Name
					break
				}
			}
		}
		return name + ": " + currentName
	}
	if len(configs) == 1 && configs[0].Boolean != nil {
		if configs[0].Boolean.CurrentValue {
			return name + ": " + i18n.Translate("On")
		}
		return name + ": " + i18n.Translate("Off")
	}
	return name
}
