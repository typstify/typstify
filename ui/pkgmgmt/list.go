package pkgmgmt

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/typst/pkg"
	appIcons "looz.ws/typstify/widgets/icons"
)

var (
	notFoundIcon      = appIcons.NewSvgIcon(appIcons.PackageOpen)
	packageSearchIcon = appIcons.NewSvgIcon(appIcons.PackageSearch)
	packageIcon       = appIcons.NewSvgIcon(appIcons.Package)
)

type PkgList struct {
	cards   []*PkgCard
	list    *widget.List
	loading bool
}

type CategoryList struct {
	categoryList *widget.List
	selection    widget.Enum
	checkedIdx   int
	clearSelect  widget.Clickable
}

func newPkgList(cards []*PkgCard, loading bool) *PkgList {
	return &PkgList{
		cards:   cards,
		loading: loading,
		list: &widget.List{
			List: layout.List{
				Axis: layout.Vertical,
			},
		},
	}
}

func newCategoryList() *CategoryList {
	return &CategoryList{
		checkedIdx: -1,
		categoryList: &widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
	}
}

func (p *PkgList) Layout(gtx C, th *theme.Theme) D {
	// Loading state
	if p.loading {
		return layout.Center.Layout(gtx, func(gtx C) D {
			lb := material.Label(th.Theme, th.TextSize, i18n.Translate("Loading packages..."))
			lb.Color = misc.WithAlpha(th.Fg, 0xb6)
			return lb.Layout(gtx)
		})
	}

	// Empty state
	if len(p.cards) <= 0 {
		return layout.Center.Layout(gtx, func(gtx C) D {
			return layout.Flex{
				Alignment: layout.Middle,
				Gap:       gtx.Dp(unit.Dp(4)),
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					return notFoundIcon.Layout(gtx, misc.WithAlpha(th.Fg, 0xb6), th.TextSize)
				}),
				layout.Rigid(func(gtx C) D {
					lb := material.Label(th.Theme, th.TextSize, i18n.Translate("No packages/templates found"))
					lb.Color = misc.WithAlpha(th.Fg, 0xb6)
					return lb.Layout(gtx)
				}),
			)

		})
	}
	return p.list.Layout(gtx, len(p.cards), func(gtx C, index int) D {
		card := p.cards[index]

		return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, func(gtx C) D {
			return card.Layout(gtx, th)
		})
	})
}

func (c *CategoryList) Layout(gtx C, th *theme.Theme) D {
	c.Update(gtx)

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return layout.Flex{
				Axis:      layout.Horizontal,
				Alignment: layout.Middle,
				Spacing:   layout.SpaceBetween,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					lb := material.Subtitle2(th.Theme, i18n.Translate("Category"))
					lb.Font.Weight = font.SemiBold
					return lb.Layout(gtx)

				}),

				layout.Rigid(func(gtx C) D {
					return material.Clickable(gtx, &c.clearSelect, func(gtx C) D {
						label := material.Label(th.Theme, th.TextSize*0.9, i18n.Translate("Clear"))
						label.Font.Style = font.Italic
						return label.Layout(gtx)
					})
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx C) D {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X

			return material.List(th.Theme, c.categoryList).Layout(gtx, len(pkg.PresetCategories),
				func(gtx C, index int) D {
					return layout.Inset{
						Bottom: unit.Dp(6),
					}.Layout(gtx, func(gtx C) D {
						checkbox := material.RadioButton(th.Theme, &c.selection, pkg.PresetCategories[index], pkg.PresetCategories[index])
						checkbox.Size = unit.Dp(th.TextSize * 1.2)
						return checkbox.Layout(gtx)
					})
				})
		}),
	)
}

func (c *CategoryList) Update(gtx C) bool {
	refresh := false
	if c.clearSelect.Clicked(gtx) {
		c.selection.Value = ""
		refresh = true
	}

	if c.selection.Update(gtx) {
		refresh = true
	}

	if refresh {
		gtx.Execute(op.InvalidateCmd{})
	}

	return refresh
}

func (c *CategoryList) GetChecked() string {
	return c.selection.Value
}
