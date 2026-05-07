package navpanel

import (
	"strings"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/widgets/icons"
)

// OutlineProvider is implemented by views that support showing a document outline.
type OutlineProvider interface {
	OutlineSymbols() []protocol.DocumentSymbol
	OnOutlineSymbolSelected(symbol protocol.DocumentSymbol)
}

type OutlineNav struct {
	providerFunc func() OutlineProvider
	list         widget.List
	clickables   []widget.Clickable
}

func NewOutlineNav() *OutlineNav {
	return &OutlineNav{
		list: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
	}
}

func (o *OutlineNav) Title() string {
	return i18n.Translate("Outline")
}

func (o *OutlineNav) LayoutHeader(gtx C, th *theme.Theme) D {
	return layout.Inset{
		Top:    unit.Dp(2),
		Bottom: unit.Dp(2),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return material.Subtitle2(th.Theme, strings.ToUpper(o.Title())).Layout(gtx)
	})
}

func (o *OutlineNav) OnClose() {}

func (o *OutlineNav) Icon() *icons.SvgIcon {
	return tocIcon
}

func (o *OutlineNav) SetProvider(p func() OutlineProvider) {
	o.providerFunc = p
}

func (o *OutlineNav) Provider() OutlineProvider {
	return o.providerFunc()
}

func (o *OutlineNav) Layout(gtx C, th *theme.Theme) D {
	if o.providerFunc == nil {
		return D{}
	}

	provider := o.providerFunc()
	if provider == nil {
		return D{}
	}

	symbols := provider.OutlineSymbols()
	if len(symbols) == 0 {
		return D{}
	}

	var items []flatSymbol
	flattenSymbols(&items, symbols, 0)

	// Ensure enough clickables.
	for len(o.clickables) < len(items) {
		o.clickables = append(o.clickables, widget.Clickable{})
	}

	list := material.List(th.Theme, &o.list)
	list.AnchorStrategy = material.Overlay

	return list.Layout(gtx, len(items), func(gtx C, index int) D {
		if o.clickables[index].Clicked(gtx) {
			provider.OnOutlineSymbolSelected(items[index].symbol)
		}
		return o.layoutItem(gtx, th, items[index], &o.clickables[index])
	})
}

type flatSymbol struct {
	symbol protocol.DocumentSymbol
	depth  int
}

func flattenSymbols(items *[]flatSymbol, symbols []protocol.DocumentSymbol, depth int) {
	for _, s := range symbols {
		*items = append(*items, flatSymbol{symbol: s, depth: depth})
		if len(s.Children) > 0 {
			flattenSymbols(items, s.Children, depth+1)
		}
	}
}

func (o *OutlineNav) layoutItem(gtx C, th *theme.Theme, item flatSymbol, btn *widget.Clickable) D {
	indent := unit.Dp(float32(item.depth)*16 + 8)

	return layout.Inset{
		Left:   indent,
		Right:  unit.Dp(8),
		Top:    unit.Dp(2),
		Bottom: unit.Dp(2),
	}.Layout(gtx, func(gtx C) D {
		return btn.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					icon := symbolIcon(item.symbol.Kind)
					return icon.Layout(gtx, misc.WithAlpha(th.Fg, 0xb6), th.TextSize*0.85)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize*0.95, item.symbol.Name)
					label.Color = th.Fg
					label.MaxLines = 1
					return label.Layout(gtx)
				}),
			)
		})
	})
}

func symbolIcon(kind protocol.SymbolKind) *icons.SvgIcon {
	switch kind {
	case protocol.Module, protocol.Namespace, protocol.Package:
		return viewListIcon
	case protocol.Class, protocol.Struct, protocol.Interface:
		return classIcon
	case protocol.Method, protocol.Function, protocol.Constructor:
		return functionIcon
	case protocol.Variable:
		return variableIcon
	default:
		return infoIcon
	}
}
