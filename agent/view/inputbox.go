package view

import (
	"image"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/addons/completion"
	gvcolor "github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"
	"github.com/sahilm/fuzzy"

	"looz.ws/typstify/agent"
)

type InputBox struct {
	*gvcode.Editor
	colorScheme *syntax.ColorScheme
	cmdPopup    *completion.CompletionPopup
	rsPopup     *completion.CompletionPopup
	submit      bool
}

func newInputBox(session *agent.ACPSession) *InputBox {
	ed := &gvcode.Editor{}

	ed.WithOptions(
		gvcode.WrapLine(true),
		gvcode.WithSoftTab(true),
		gvcode.WithCornerRadius(unit.Dp(4)),
		gvcode.WithTabWidth(2),
	)

	cm := &completion.DefaultCompletion{Editor: ed}
	cmdCompletor := &commandCompletor{session: session}
	rsCompletor := &resourceCompletor{session: session}

	cmdPopup := completion.NewCompletionPopup(ed, cm)
	rsPopup := completion.NewCompletionPopup(ed, cm)
	cm.AddCompletor(rsCompletor, rsPopup)
	cm.AddCompletor(cmdCompletor, cmdPopup)

	ed.WithOptions(gvcode.WithAutoCompletion(cm))

	b := &InputBox{
		Editor:   ed,
		cmdPopup: cmdPopup,
		rsPopup:  rsPopup,
	}

	ed.RegisterCommand("input-box",
		key.Filter{Name: key.NameEnter, Required: key.ModShift},
		func(gtx layout.Context, evt key.Event) gvcode.EditorEvent {
			b.submit = true
			return nil
		})

	ed.RegisterCommand("input-box",
		key.Filter{Name: key.NameReturn, Required: key.ModShift},
		func(gtx layout.Context, evt key.Event) gvcode.EditorEvent {
			b.submit = true
			return nil
		})

	return b
}

func (b *InputBox) Update(gtx C) bool {
	for {
		_, ok := b.Editor.Update(gtx)
		if !ok {
			break
		}
	}

	submitted := b.submit
	b.submit = false
	return submitted
}

func (b *InputBox) Layout(gtx C, th *theme.Theme) D {
	cs := syntax.ColorScheme{}
	cs.Background = gvcolor.MakeColor(th.Bg)
	cs.Foreground = gvcolor.MakeColor(th.Fg)
	cs.SelectColor = gvcolor.MakeColor(th.ContrastBg).MulAlpha(th.SelectedAlpha)
	b.Editor.WithOptions(
		gvcode.WithFont(font.Font{Typeface: th.Face}),
		gvcode.WithTextSize(th.TextSize),
		gvcode.WithTextAlignment(text.Start),
		gvcode.WithLineHeight(0, 1.5),
		gvcode.WithColorScheme(cs),
	)

	gtx.Constraints.Max.Y = min(gtx.Dp(unit.Dp(120)), gtx.Constraints.Max.Y)
	popupSize := image.Point{
		X: int(float32(gtx.Constraints.Max.X) * 0.8),
		Y: int(float32(gtx.Constraints.Max.Y) * 0.9),
	}
	b.cmdPopup.Theme = th.Theme
	b.cmdPopup.Size = popupSize
	b.cmdPopup.TextSize = th.TextSize
	b.rsPopup.Theme = th.Theme
	b.rsPopup.Size = popupSize
	b.rsPopup.TextSize = th.TextSize

	return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx C) D {
		return b.Editor.Layout(gtx, th.Shaper)
	})

}

var _ gvcode.Completor = (*commandCompletor)(nil)
var _ gvcode.Completor = (*resourceCompletor)(nil)

type commandCompletor struct {
	session *agent.ACPSession
}

func (c *commandCompletor) Trigger() gvcode.Trigger {
	return gvcode.Trigger{
		Characters: []string{"/"},
		Policy:     completion.ExplicitTriggerPolicy{},
	}
}

func (c *commandCompletor) Suggest(ctx gvcode.CompletionContext) []gvcode.CompletionCandidate {
	if c.session == nil {
		return nil
	}
	commands := c.session.AvailableCommands()
	candidates := make([]gvcode.CompletionCandidate, 0, len(commands))
	for _, cmd := range commands {
		candidates = append(candidates, gvcode.CompletionCandidate{
			Label:       "/" + cmd.Name,
			TextEdit:    gvcode.TextEdit{NewText: cmd.Name},
			Description: cmd.Description,
			Kind:        "snippet",
		})
	}
	return candidates
}

func (c *commandCompletor) FilterAndRank(pattern string, candidates []gvcode.CompletionCandidate) []gvcode.CompletionCandidate {
	if pattern == "" {
		return candidates
	}
	filtered := candidates[:0]
	lower := strings.ToLower(pattern)
	for _, cand := range candidates {
		if strings.Contains(strings.ToLower(cand.Label), lower) {
			filtered = append(filtered, cand)
		}
	}
	return filtered
}

type resourceCompletor struct {
	session *agent.ACPSession
}

func (r *resourceCompletor) Trigger() gvcode.Trigger {
	return gvcode.Trigger{
		Characters: []string{"@"},
		Policy:     completion.ExplicitTriggerPolicy{},
	}
}

func (r *resourceCompletor) Suggest(ctx gvcode.CompletionContext) []gvcode.CompletionCandidate {
	if r.session == nil {
		return nil
	}

	root := r.session.Cwd
	candidates := []gvcode.CompletionCandidate{}

	err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return nil
		}

		resourcePath := filepath.ToSlash(rel)
		kind := "file"
		if e.IsDir() {
			kind = "folder"
			resourcePath += "/"
		}
		candidates = append(candidates, gvcode.CompletionCandidate{
			Label:    resourcePath,
			TextEdit: gvcode.TextEdit{NewText: resourcePath},
			Kind:     kind,
		})
		return nil
	})
	if err != nil {
		return nil
	}
	return candidates
}

type resourceCandidateSource struct {
	candidates []gvcode.CompletionCandidate
}

func (src *resourceCandidateSource) String(i int) string {
	return src.candidates[i].Label
}

func (src *resourceCandidateSource) Len() int {
	return len(src.candidates)
}

func (r *resourceCompletor) FilterAndRank(pattern string, candidates []gvcode.CompletionCandidate) []gvcode.CompletionCandidate {
	if pattern == "" {
		return candidates
	}

	pattern = strings.ToLower(strings.TrimLeft(strings.ReplaceAll(pattern, "\\", "/"), "/"))
	if pattern == "" || strings.Contains(pattern, "..") {
		return nil
	}

	matches := fuzzy.FindFrom(pattern, &resourceCandidateSource{candidates: candidates})
	filtered := make([]gvcode.CompletionCandidate, 0, len(matches))
	for _, match := range matches {
		filtered = append(filtered, candidates[match.Index])
	}
	return filtered
}
