package navpanel

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets"
	"looz.ws/typstify/widgets/icons"
)

// OutlineProvider is implemented by views that support showing a document outline.
type OutlineProvider interface {
	OutlineSymbols() []protocol.DocumentSymbol
	OnOutlineSymbolSelected(symbol protocol.DocumentSymbol)
	CaretLine() int
}

type OutlineNav struct {
	providerFunc func() OutlineProvider
	list         widget.List
	clickables   []*widgets.InteractiveLabel
	selectedIdx  int
	activeIdx    int
	expanded     map[string]bool
	toggleBtns   map[string]*widget.Clickable
}

func NewOutlineNav() *OutlineNav {
	return &OutlineNav{
		list: widget.List{
			List: layout.List{Axis: layout.Vertical},
		},
		expanded:   make(map[string]bool),
		toggleBtns: make(map[string]*widget.Clickable),
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
	flattenSymbols(&items, o.expanded, symbols, 0)

	// Bi-directional sync: scroll the outline to the symbol under the caret.
	o.syncActiveIndex(o.findActiveIndex(items, provider.CaretLine()))

	list := material.List(th.Theme, &o.list)
	list.AnchorStrategy = material.Overlay
	list.ScrollbarStyle = utils.MakeScrollbar(th.Theme, list.Scrollbar, misc.WithAlpha(th.Fg, 0x30))

	return list.Layout(gtx, len(items), func(gtx C, index int) D {
		// When ScrollTo jumped the list to index 6, the slice is too short, as List start painting at index 6.
		// Using for loop to keep appending until it catches up.
		for len(o.clickables) <= index {
			o.clickables = append(o.clickables, &widgets.InteractiveLabel{})
		}

		label := o.clickables[index]
		item := items[index]

		// Process chevron toggle click first — fires before the label's
		// click gesture because it is registered later in draw order.
		toggleClicked := false
		if item.hasChildren {
			if _, exists := o.toggleBtns[item.key]; !exists {
				o.toggleBtns[item.key] = &widget.Clickable{}
			}
			if o.toggleBtns[item.key].Clicked(gtx) {
				o.expanded[item.key] = !item.expanded
				gtx.Execute(op.InvalidateCmd{})
				toggleClicked = true
			}
		}

		// Set visual state before layout so background renders correctly.
		if index == o.selectedIdx {
			label.Select()
		} else {
			label.Unselect()
		}

		// Always call Update to consume pointer events (enter/leave/click).
		// Only navigate when the click did not land on the toggle chevron.
		if label.Update(gtx) && !toggleClicked {
			provider.OnOutlineSymbolSelected(item.symbol)

			if o.selectedIdx != index && o.selectedIdx < len(o.clickables) && o.clickables[o.selectedIdx].IsSelected() {
				o.clickables[o.selectedIdx].Unselect()
			}
			o.selectedIdx = index
		}

		dims := o.layoutItem(gtx, th, item, label)

		// Re-apply selection after Layout (which calls Update internally)
		// so click-driven changes don't override caret-driven selection.
		if index == o.selectedIdx {
			label.Select()
		} else {
			label.Unselect()
		}

		return dims
	})
}

// findActiveIndex returns the index of the symbol that contains the caret line.
// Each symbol's effective range is [start, start of next sibling), where "next
// sibling" means the first subsequent item at the same or higher depth level.
// This partitions the document without gaps or overlaps, and the deepest match
// wins because children follow their parent in the flattened list.
func (o *OutlineNav) findActiveIndex(items []flatSymbol, caretLine int) int {
	idx := -1
	for i, item := range items {
		symStart := int(item.symbol.Range.Start.Line)
		symEnd := int(^uint(0) >> 1) // max int
		for j := i + 1; j < len(items); j++ {
			if items[j].depth <= item.depth {
				symEnd = int(items[j].symbol.Range.Start.Line)
				break
			}
		}
		if caretLine >= symStart && caretLine < symEnd {
			idx = i
		}
	}
	return idx
}

// syncActiveIndex updates the active symbol, scrolls to it, and syncs the
// click-driven selection so that caret movement in the editor is reflected
// in the outline panel.
func (o *OutlineNav) syncActiveIndex(newActive int) {
	if newActive == o.activeIdx {
		return
	}
	o.activeIdx = newActive
	if newActive >= 0 {
		o.list.ScrollTo(newActive)
		o.selectedIdx = newActive
	}
}

type flatSymbol struct {
	symbol      protocol.DocumentSymbol
	depth       int
	hasChildren bool
	expanded    bool
	key         string
}

func symbolKey(s protocol.DocumentSymbol) string {
	return fmt.Sprintf("%s-%d-%d", s.Name, s.SelectionRange.Start.Line, s.SelectionRange.Start.Character)
}

func flattenSymbols(items *[]flatSymbol, expanded map[string]bool, symbols []protocol.DocumentSymbol, depth int) {
	for _, s := range symbols {
		key := symbolKey(s)
		hasChildren := len(s.Children) > 0
		exp := hasChildren
		if hasChildren {
			if v, ok := expanded[key]; ok {
				exp = v
			}
		}

		*items = append(*items, flatSymbol{
			symbol:      s,
			depth:       depth,
			hasChildren: hasChildren,
			expanded:    exp,
			key:         key,
		})

		if hasChildren && exp {
			flattenSymbols(items, expanded, s.Children, depth+1)
		}
	}
}

var (
	chevronRightIcon = icons.NewSvgIcon(icons.ChevronRight)
	chevronDownIcon  = icons.NewSvgIcon(icons.ChevronDown)
)

func (o *OutlineNav) layoutItem(gtx C, th *theme.Theme, item flatSymbol, btn *widgets.InteractiveLabel) D {
	indent := unit.Dp(float32(item.depth)*16 + 8)

	return btn.Layout(gtx, th, func(gtx C, textColor color.NRGBA) D {
		return layout.Inset{
			Left:   indent,
			Right:  unit.Dp(8),
			Top:    unit.Dp(3),
			Bottom: unit.Dp(3),
		}.Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					if !item.hasChildren {
						return layout.Spacer{Width: unit.Dp(16)}.Layout(gtx)
					}
					toggleBtn := o.toggleBtns[item.key]
					return toggleBtn.Layout(gtx, func(gtx C) D {
						chevron := chevronRightIcon
						if item.expanded {
							chevron = chevronDownIcon
						}
						return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
							return chevron.Layout(gtx, misc.WithAlpha(textColor, 0xb6), th.TextSize*0.85)
						})
					})
				}),
				layout.Rigid(func(gtx C) D {
					icon := symbolIcon(item.symbol.Kind)
					return icon.Layout(gtx, misc.WithAlpha(th.Fg, 0xb6), th.TextSize*0.85)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx C) D {
					label := material.Label(th.Theme, th.TextSize*0.9, item.symbol.Name)
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
