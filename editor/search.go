package editor

import (
	stdColor "image/color"
	"regexp"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	cmp "gioui.org/x/component"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	gw "github.com/oligo/gioview/widget"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/decoration"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/widgets/icons"
)

var (
	searchIcon      = icons.NewSvgIcon(icons.Search)
	closeSearchIcon = icons.NewSvgIcon(icons.X)
	backwardIcon    = icons.NewSvgIcon(icons.ArrowUp)
	forwardIcon     = icons.NewSvgIcon(icons.ArrowDown)
	dropdownIcon    = icons.NewSvgIcon(icons.ChevronDown)
	dropupIcon      = icons.NewSvgIcon(icons.ChevronUp)
)

const inputBarWidth = unit.Dp(200)

// Match option for text match
type MatchOption uint8

const (
	CaseSensitive MatchOption = 1 << iota
	WholeWorld
	UseRegex
)

type textSearch struct {
	options MatchOption
	substr  string
}

type TextSearchBar struct {
	editor       *gvcode.Editor
	search       textSearch
	matches      []gvcode.TextRange
	matchCount   int
	currentMatch int

	searchInput    gw.TextField
	replaceInput   gw.TextField
	prevBtn        widget.Clickable
	nextBtn        widget.Clickable
	closeBtn       widget.Clickable
	showReplaceBtn widget.Clickable
	replaceNextBtn widget.Clickable
	replaceAllBtn  widget.Clickable

	matchCaseChecked widget.Bool
	wholeWordChecked widget.Bool
	useRegexChecked  widget.Bool
	showReplace      bool
	anim             *cmp.VisibilityAnimation
}

func (sb *TextSearchBar) Update(gtx C) {
	if sb.anim == nil {
		sb.anim = &cmp.VisibilityAnimation{
			State:    cmp.Invisible,
			Duration: time.Millisecond * 150,
		}
	}

	// key and pointer handler.
	// Check if searchBar is visible first to prevent it competes with gioview view.ModalView.
	if sb.anim.Visible() {
		for {
			e, ok := gtx.Event(
				//key.Filter{Focus: sb.searchInput.State(), Name: key.NameEscape},
				//key.Filter{Focus: sb.replaceInput.State(), Name: key.NameEscape},
				key.Filter{Name: key.NameEscape}, // global key handler, without a focused target.
			)
			if !ok {
				break
			}

			switch event := e.(type) {
			case key.Event:
				if event.Name == key.NameEscape {
					sb.Hide(gtx)
				}
			}
		}
	}

	if sb.closeBtn.Clicked(gtx) {
		sb.anim.Disappear(gtx.Now)
	}
	if sb.prevBtn.Clicked(gtx) {
		sb.moveToPrevMatch()
	}
	if sb.nextBtn.Clicked(gtx) {
		sb.moveToNextMatch()
	}

	// handle find events.
	sb.searchOnUpdate(gtx)

	// handle replace events
	if sb.showReplaceBtn.Clicked(gtx) {
		sb.showReplace = !sb.showReplace
	}

	// replace a single match
	if sb.replaceNextBtn.Clicked(gtx) && sb.matchCount > 0 {
		// In case we lost current match highlight
		sb.nextMatch(sb.currentMatch)
		sb.editor.Insert(sb.replaceInput.Text())
		// offset may have changed, update match ranges in editor.
		sb.doSearch(&sb.search)
	}

	// replace all matches
	if sb.replaceAllBtn.Clicked(gtx) {
		sb.replaceAll()
	}
}

func (sb *TextSearchBar) Show(gtx C) {
	sb.anim.Appear(gtx.Now)
	sb.searchInput.SetFocus(gtx)
	if sb.editor.SelectionLen() > 0 {
		sb.searchInput.SetText(sb.editor.SelectedText())
	}
}

func (sb *TextSearchBar) Hide(gtx C) {
	sb.anim.Disappear(gtx.Now)
	// let lastTag focus agnain.
	gtx.Execute(key.FocusCmd{Tag: sb.editor})
}

func (sb *TextSearchBar) Visible() bool {
	return sb.anim != nil && sb.anim.Visible()
}

func (sb *TextSearchBar) ReSearch() {
	sb.doSearch(&sb.search)
}

func (sb *TextSearchBar) moveToPrevMatch() {
	sb.currentMatch -= 1
	if sb.currentMatch < 0 {
		sb.currentMatch = sb.matchCount - 1
	}
	sb.nextMatch(sb.currentMatch)
}

func (sb *TextSearchBar) moveToNextMatch() {
	sb.currentMatch += 1
	if sb.currentMatch > sb.matchCount-1 {
		sb.currentMatch = 0
	}
	sb.nextMatch(sb.currentMatch)
}

// NextMatch is used to switch between the [MatchRange]s. This also selects the next match and causes
// the selection background drawn under the matched text.
func (sb *TextSearchBar) nextMatch(index int) {
	if index < 0 || index >= len(sb.matches) {
		return
	}

	sb.currentMatch = index
	sb.editor.SetCaret(sb.matches[sb.currentMatch].Start, sb.matches[sb.currentMatch].End)
}

func (sb *TextSearchBar) searchOnUpdate(gtx C) {
	updated := false
	if sb.searchInput.Changed() {
		sb.search.substr = sb.searchInput.Text()
		updated = true
	}

	if sb.matchCaseChecked.Update(gtx) {
		if sb.matchCaseChecked.Value {
			sb.search.options |= CaseSensitive
		} else {
			// clear CaseSensitive bits
			sb.search.options &= ^CaseSensitive
		}
		updated = true
	}

	if sb.wholeWordChecked.Update(gtx) {
		if sb.wholeWordChecked.Value {
			sb.search.options |= WholeWorld
		} else {
			sb.search.options &= ^WholeWorld
		}
		updated = true
	}

	if sb.useRegexChecked.Update(gtx) {
		if sb.useRegexChecked.Value {
			sb.search.options |= UseRegex
		} else {
			sb.search.options &= ^UseRegex
		}
		updated = true
	}

	if updated {
		sb.doSearch(&sb.search)
	}

	if (sb.anim.State == cmp.Disappearing || sb.anim.State == cmp.Invisible) && sb.search.substr != "" {
		sb.clear()
	}

}

func (sb *TextSearchBar) doSearch(search *textSearch) {
	if search != nil {
		err := sb.findText(sb.editor.Text(), search.substr, search.options)
		if err == nil {
			sb.editor.ClearDecorations("editor-search")
			//sb.editor.SetHighlights(sb.matches)
			decorations := make([]decoration.Decoration, 0)
			for _, match := range sb.matches {
				decorations = append(decorations, decoration.Decoration{
					Source: "editor-search",
					Start:  match.Start,
					End:    match.End,
					Background: &decoration.Background{
						Color: color.MakeColor(stdColor.NRGBA{R: 255, G: 100, B: 100, A: 0x96}),
					},
					Border: &decoration.Border{
						Color: color.MakeColor(stdColor.NRGBA{R: 255, G: 100, B: 100, A: 0xf6}),
					},
				})
			}
			sb.editor.AddDecorations(decorations...)
		}
		sb.matchCount = len(sb.matches)
		sb.currentMatch = 0
	} else {
		sb.editor.ClearDecorations("editor-search")
	}
}

func (sb *TextSearchBar) clear() {
	sb.search = textSearch{}
	sb.doSearch(nil)
	sb.searchInput.Clear()
	sb.replaceInput.Clear()
	sb.showReplace = false
	sb.matchCaseChecked.Value = false
	sb.wholeWordChecked.Value = false
	sb.useRegexChecked.Value = false
}

// Perform replacements from the last match to the first to avoid offset shifting.
func (sb *TextSearchBar) replaceAll() {
	if sb.matchCount <= 0 {
		return
	}
	replaceStr := sb.replaceInput.Text()
	sb.editor.ReplaceAll(sb.matches, replaceStr)
	sb.doSearch(&sb.search)
}

func (sb *TextSearchBar) Layout(gtx C, th *theme.Theme) D {
	sb.Update(gtx)

	if !sb.anim.Visible() {
		return D{}
	}

	macro := op.Record(gtx.Ops)
	dims := sb.layout(gtx, th)
	callOp := macro.Stop()

	if sb.anim.Animating() {
		dims.Size.Y = int(float32(dims.Size.Y) * sb.anim.Revealed(gtx))
	}

	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, sb)
	callOp.Add(gtx.Ops)

	return dims
}

func (sb *TextSearchBar) layout(gtx C, th *theme.Theme) D {
	return widget.Border{
		Color:        misc.WithAlpha(th.Fg, 0xb6),
		CornerRadius: unit.Dp(4),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, func(gtx C) D {
					return material.Clickable(gtx, &sb.showReplaceBtn, func(gtx C) D {
						icon := dropdownIcon
						if sb.showReplace {
							icon = dropupIcon
						}
						return icon.Layout(gtx, th.Fg, th.TextSize)
					})
				})
			}),
			layout.Rigid(func(gtx C) D {
				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return sb.layoutSearchBar(gtx, th)
					}),
					layout.Rigid(func(gtx C) D {
						if !sb.showReplace {
							return D{}
						}
						return sb.layoutReplaceBar(gtx, th)
					}),
				)
			}),
		)
	})
}

func (sb *TextSearchBar) layoutSearchBar(gtx C, th *theme.Theme) D {

	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{
					Right: unit.Dp(8),
				}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = gtx.Dp(inputBarWidth)
					sb.searchInput.SingleLine = true
					sb.searchInput.LabelOption = gw.LabelOption{Alignment: gw.Hidden}
					sb.searchInput.Padding = unit.Dp(6)
					// sb.searchInput.Leading = func(gtx C) D {
					// 	return misc.Icon{Icon: RegexIcon, Size: unit.Dp(18), Color: misc.WithAlpha(th.Fg, 0xb0)}.Layout(gtx, th)
					// }

					return sb.searchInput.Layout(gtx, th, "Find")
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx C) D {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						checkbox := material.CheckBox(th.Theme, &sb.matchCaseChecked, i18n.Translate("Match Case"))
						checkbox.Size = unit.Dp(th.TextSize * 1.2)
						return checkbox.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						checkbox := material.CheckBox(th.Theme, &sb.wholeWordChecked, i18n.Translate("Whole Word"))
						checkbox.Size = unit.Dp(th.TextSize * 1.2)
						return checkbox.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						checkbox := material.CheckBox(th.Theme, &sb.useRegexChecked, i18n.Translate("Use Regex"))
						checkbox.Size = unit.Dp(th.TextSize * 1.2)
						return checkbox.Layout(gtx)
					}),
				)
			}),

			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(60))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				if sb.matchCount <= 0 {
					return material.Label(th.Theme, th.TextSize, i18n.Translate("No results")).Layout(gtx)
				}
				lb := material.Label(th.Theme, th.TextSize, i18n.Translate("%d of %d", sb.currentMatch+1, sb.matchCount))
				return lb.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &sb.prevBtn, func(gtx C) D {
					return backwardIcon.Layout(gtx, th.Fg, th.TextSize)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &sb.nextBtn, func(gtx C) D {
					return forwardIcon.Layout(gtx, th.Fg, th.TextSize)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx C) D {
				return material.Clickable(gtx, &sb.closeBtn, func(gtx C) D {
					return closeSearchIcon.Layout(gtx, th.Fg, th.TextSize)
				})
			}),
		)
	})
}

func (sb *TextSearchBar) layoutReplaceBar(gtx C, th *theme.Theme) D {
	return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				return layout.Inset{
					Right: unit.Dp(8),
				}.Layout(gtx, func(gtx C) D {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(200))
					sb.replaceInput.SingleLine = true
					sb.replaceInput.LabelOption = gw.LabelOption{Alignment: gw.Hidden}
					sb.replaceInput.Padding = unit.Dp(6)
					return sb.replaceInput.Layout(gtx, th, "Replace")
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx C) D {
				if sb.matchCount <= 0 {
					gtx = gtx.Disabled()
				}

				btn := material.Button(th.Theme, &sb.replaceNextBtn, i18n.Translate("Replace"))
				btn.Inset = layout.UniformInset(unit.Dp(4))
				btn.Background = th.Bg
				btn.Color = th.Fg
				return btn.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),

			layout.Rigid(func(gtx C) D {

				btn := material.Button(th.Theme, &sb.replaceAllBtn, i18n.Translate("Replace All"))
				btn.Inset = layout.UniformInset(unit.Dp(4))
				btn.Background = th.Bg
				btn.Color = th.Fg
				return btn.Layout(gtx)
			}),
		)
	})
}

func (sb *TextSearchBar) findText(main string, substring string, options MatchOption) error {
	sb.matches = sb.matches[:0]

	if substring == "" {
		return nil
	}

	matchCase := options&CaseSensitive > 0
	wholeWordMatch := options&WholeWorld > 0
	useRegex := options&UseRegex > 0
	//log.Printf("options: %08b, matchCase: %08b, wholeWord: %08b, useRE: %08b", options, options&CaseSensitive, options&WholeWorld, options&UseRegex)

	if useRegex {
		if wholeWordMatch {
			substring = `\b` + substring + `\b`
		}
		if !matchCase {
			substring = "(?i)" + substring
		}
		//log.Println("final re: ", substring)
		re, err := regexp.Compile(substring)
		if err != nil {
			return err
		}

		matches := re.FindAllIndex([]byte(main), -1)
		for _, match := range matches {
			sb.matches = append(sb.matches, gvcode.TextRange{
				Start: byteToRuneIndex(main, match[0]),
				End:   byteToRuneIndex(main, match[1]),
			})
		}
	} else {
		if wholeWordMatch {
			substring = `\b` + substring + `\b`
		}
		if !matchCase {
			substring = "(?i)" + substring
		}

		re, err := regexp.Compile(substring)
		if err != nil {
			return err
		}

		matches := re.FindAllIndex([]byte(main), -1)
		for _, match := range matches {
			sb.matches = append(sb.matches, gvcode.TextRange{
				Start: byteToRuneIndex(main, match[0]),
				End:   byteToRuneIndex(main, match[1]),
			})
		}
	}

	return nil
}
