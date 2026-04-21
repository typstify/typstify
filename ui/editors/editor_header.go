package editors

import (
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	appIcons "looz.ws/typstify/widgets/icons"
)

var (
	headerChevronIcon = appIcons.NewSvgIcon(appIcons.ChevronRight)
)

type editorHeaderAction struct {
	Name      string
	Icon      *appIcons.SvgIcon
	OnClicked func(gtx C)
}

type ViewActionState struct {
	btn widget.Clickable
	//tip wg.TipArea
}

type editorHeader struct {
	rootDir     string
	currentPath string
	segments    []breadcrumbSegment
	pathList    layout.List
	actions     []editorHeaderAction
	actionState []ViewActionState
}

type breadcrumbSegment struct {
	label   string
	current bool
}

func newEditorHeader(rootDir, currentPath string, actions []editorHeaderAction) *editorHeader {
	eh := &editorHeader{
		rootDir: rootDir,
		actions: actions,
	}
	eh.pathList.Axis = layout.Horizontal
	eh.actionState = make([]ViewActionState, len(actions))
	eh.SetCurrentPath(currentPath)
	return eh
}

func (eh *editorHeader) SetCurrentPath(path string) {
	if eh.currentPath == path {
		return
	}

	eh.currentPath = path
	eh.segments = buildBreadcrumbSegments(eh.rootDir, path)
}

func (eh *editorHeader) Layout(gtx C, th *theme.Theme) D {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, misc.WithAlpha(th.Bg2, 0xe8), clip.Rect{Max: gtx.Constraints.Max}.Op())

	return layout.Inset{
		Left:   unit.Dp(12),
		Right:  unit.Dp(12),
		Top:    unit.Dp(4),
		Bottom: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
			Spacing:   layout.SpaceBetween,
		}.Layout(gtx,
			layout.Flexed(1, func(gtx C) D {
				return eh.layoutSegments(gtx, th)
			}),
			layout.Rigid(func(gtx C) D {
				return eh.layoutActions(gtx, th)
			}),
		)
	})
}

func (eh *editorHeader) layoutSegments(gtx C, th *theme.Theme) D {
	if len(eh.segments) == 0 {
		return D{}
	}

	return eh.pathList.Layout(gtx, len(eh.segments), func(gtx C, index int) D {
		segment := eh.segments[index]
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				label := material.Label(th.Theme, th.TextSize*0.9, segment.label)
				if segment.current {
					label.Color = th.Fg
				} else {
					label.Color = misc.WithAlpha(th.Fg, 0x9d)
				}
				return label.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				if index == len(eh.segments)-1 {
					return D{}
				}
				return layout.Inset{
					Left:  unit.Dp(3),
					Right: unit.Dp(3),
				}.Layout(gtx, func(gtx C) D {
					return headerChevronIcon.Layout(gtx, misc.WithAlpha(th.Fg, 0x72), th.TextSize)
				})
			}),
		)
	})
}

func (eh *editorHeader) layoutActions(gtx C, th *theme.Theme) D {
	if len(eh.actions) == 0 {
		return D{}
	}

	children := make([]layout.FlexChild, 0, len(eh.actions)*2)
	for i := range eh.actions {
		action := eh.actions[i]
		state := &eh.actionState[i]

		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
		}

		children = append(children, layout.Rigid(func(gtx C) D {
			if state.btn.Clicked(gtx) && action.OnClicked != nil {
				action.OnClicked(gtx)
			}

			return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx C) D {
				return state.btn.Layout(gtx, func(gtx C) D {
					iconColor := th.Fg
					if state.btn.Hovered() {
						iconColor = th.ContrastBg
					}
					return action.Icon.Layout(gtx, iconColor, th.TextSize)
				})
			})

		}))
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx, children...)
}

func buildBreadcrumbSegments(rootDir, currentPath string) []breadcrumbSegment {
	if currentPath == "" {
		return nil
	}

	if rel, ok := relativePath(rootDir, currentPath); ok {
		segments := splitBreadcrumbPath(rel)
		if len(segments) == 0 {
			segments = []string{filepath.Base(currentPath)}
		}

		rootLabel := filepath.Base(rootDir)
		if rootLabel == "" {
			rootLabel = rootDir
		}

		return appendBreadcrumbSegments(rootLabel, segments)
	}

	return appendBreadcrumbSegments("", splitBreadcrumbPath(filepath.Clean(currentPath)))
}

func appendBreadcrumbSegments(root string, parts []string) []breadcrumbSegment {
	segments := make([]breadcrumbSegment, 0, len(parts)+1)
	if root != "" && root != "." {
		segments = append(segments, breadcrumbSegment{label: root})
	}

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		segments = append(segments, breadcrumbSegment{label: part})
	}

	if len(segments) == 0 {
		segments = append(segments, breadcrumbSegment{label: filepath.Base(root)})
	}

	segments[len(segments)-1].current = true
	return segments
}

func relativePath(rootDir, currentPath string) (string, bool) {
	if rootDir == "" {
		return "", false
	}

	rel, err := filepath.Rel(rootDir, currentPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}

	return rel, true
}

func splitBreadcrumbPath(path string) []string {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	if volume != "" {
		clean = strings.TrimPrefix(clean, volume)
	}

	clean = strings.TrimPrefix(clean, string(filepath.Separator))
	parts := strings.Split(clean, string(filepath.Separator))
	if volume != "" {
		return append([]string{volume}, parts...)
	}

	if filepath.IsAbs(path) {
		return append([]string{string(filepath.Separator)}, parts...)
	}

	return parts
}
