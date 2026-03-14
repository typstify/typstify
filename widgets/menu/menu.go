package menu

import (
	"github.com/oligo/gioview/theme"

	"gioui.org/widget"
)

type MenuState struct {
	Options        [][]MenuOption
	OptionStates   []*widget.Clickable
	FocusedOption  int
	RequestDismiss bool
}

type MenuOption struct {
	Layout    func(gtx C, th *theme.Theme) D
	OnClicked func(gtx C) error
}

func (m *MenuState) SetOptions(options [][]MenuOption) {
	m.FocusedOption = -1
	m.RequestDismiss = false
	m.Options = options
	m.OptionStates = m.OptionStates[:0]

	idx := 0
	for _, group := range options {
		for range group {
			if len(m.OptionStates) < idx+1 {
				m.OptionStates = append(m.OptionStates, &widget.Clickable{})
			}

			idx++
		}
	}
}
