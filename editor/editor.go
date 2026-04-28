package editor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/x/component"
	"github.com/fsnotify/fsnotify"
	"github.com/oligo/gioview/menu"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gvcode"
	"github.com/oligo/gvcode/addons/completion"
	gvcolor "github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/gutter/providers"
	"github.com/oligo/gvcode/textstyle/decoration"
	"github.com/oligo/gvcode/textstyle/syntax"

	"looz.ws/typstify/lsp"
	"looz.ws/typstify/lsp/protocol"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/utils"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var errHighlightColor, _ = gvcolor.Hex2Color("#e74c3c")

type TextEditor struct {
	state        *gvcode.Editor
	filename     string // for syntax highlight
	originalHash string
	autoSaver    *AutoSaver
	highlighter  *Highlighter
	colorScheme  *syntax.ColorScheme
	wrapLine     bool

	contextMenu *menu.ContextMenu
	//editorConf  *editor.EditorConf
	status    *EditorStatus
	searchbar *TextSearchBar
	xScroll   widget.Scrollbar
	yScroll   widget.Scrollbar

	// features relying on LSP.
	lspClient *lsp.Client
	hoverTips *HoverTips
	popup     *completion.CompletionPopup
	// diagnostics decorations
	diagnosticsDecos []decoration.Decoration
	overviewRuler    OverviewRuler

	diffProvider          *providers.VCSDiffProvider
	differ                *GitDiff
	pendingExternalChange atomic.Bool
	srv                   *service.ServiceFacade

	OnSelectChange func(gvcode.Position)
	OnOpenLink     func(link string, external bool)
}

func (me *TextEditor) File() string {
	return me.filename
}

func (me *TextEditor) Layout(gtx layout.Context, th *theme.Theme, settings *settings.EditorSettings) layout.Dimensions {
	if me.contextMenu == nil {
		me.contextMenu = menu.NewContextMenu(EditorMenuOptions(gtx, me), false)
		// me.contextMenu.Background = misc.WithAlpha(th.Fg, th.HoverAlpha)
		me.contextMenu.MaxWidth = min(unit.Dp(230), unit.Dp(gtx.Constraints.Max.X/int(gtx.Metric.PxPerDp)))
	}

	if me.searchbar == nil {
		me.searchbar = &TextSearchBar{editor: me.state}
	}

	me.update(gtx, th, settings)
	dims := me.layoutEditor(gtx, th, settings)
	me.contextMenu.Layout(gtx, th)
	return dims
}

// layout the editor with a background layer to handle shortcut key events
func (me *TextEditor) layoutEditor(gtx C, th *theme.Theme, settings *settings.EditorSettings) D {
	var editorDims D

	scrollIndicatorColor := me.colorScheme.Foreground.MulAlpha(0x30)

	editorOps := func() op.CallOp {
		macro := op.Record(gtx.Ops)
		editorDims = func(gtx C) D {
			return layout.Flex{
				Axis: layout.Vertical,
			}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					// Layout search bar on top of the editor
					return layout.Inset{Right: unit.Dp(16)}.Layout(gtx, func(gtx C) D {
						return me.layoutSearchBar(gtx, th)
					})
				}),
				layout.Flexed(1, func(gtx C) D {
					return layout.Flex{
						Axis: layout.Horizontal,
					}.Layout(gtx,
						layout.Flexed(1.0, func(gtx layout.Context) layout.Dimensions {
							me.state.WithOptions(
								gvcode.WithFont(font.Font{Typeface: font.Typeface(settings.TypeFace), Weight: font.Weight(settings.Weight)}),
								gvcode.WithTextSize(unit.Sp(settings.TextSize)),
								gvcode.WithLineHeight(0, settings.LineHeightScale),
							)

							dims := me.state.Layout(gtx, th.Shaper)

							// draw overlay
							me.state.PaintOverlay(gtx, me.hoverTips.Pos(), func(gtx layout.Context) layout.Dimensions {
								return me.hoverTips.Layout(gtx, th)
							})

							macro := op.Record(gtx.Ops)
							scrollbarDims := func(gtx C) D {
								return layout.Inset{
									Left: gtx.Metric.PxToDp(me.state.GutterWidth()),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									minX, maxX, _, _ := me.state.ScrollRatio()
									bar := utils.MakeScrollbar(th.Theme, &me.xScroll, scrollIndicatorColor.NRGBA())
									return bar.Layout(gtx, layout.Horizontal, minX, maxX)
								})
							}(gtx)

							scrollbarOp := macro.Stop()
							defer op.Offset(image.Point{Y: dims.Size.Y - scrollbarDims.Size.Y}).Push(gtx.Ops).Pop()
							scrollbarOp.Add(gtx.Ops)

							return dims
						}),

						layout.Rigid(func(gtx C) D {
							me.overviewRuler.Layout(gtx, th)

							_, _, minY, maxY := me.state.ScrollRatio()
							bar := utils.MakeScrollbar(th.Theme, &me.yScroll, scrollIndicatorColor.NRGBA())
							return bar.Layout(gtx, layout.Vertical, minY, maxY)
						}),
					)

				}),
			)
		}(gtx)

		return macro.Stop()
	}()

	defer clip.Rect(image.Rectangle{Max: editorDims.Size}).Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, me)
	editorOps.Add(gtx.Ops)

	return editorDims
}

func (me *TextEditor) layoutSearchBar(gtx C, th *theme.Theme) D {
	gtx.Constraints.Min = image.Point{}
	marco := op.Record(gtx.Ops)
	dims := me.searchbar.Layout(gtx, th)
	callOp := marco.Stop()

	if !me.searchbar.Visible() {
		return D{}
	}

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, th.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// surface return zero dimensions, so we have to use the inner widget's dims here.
	defer op.Offset(image.Point{X: gtx.Constraints.Max.X - dims.Size.X}).Push(gtx.Ops).Pop()
	surface := component.Surface(th.Theme)
	surface.CornerRadius = unit.Dp(4)
	surface.Layout(gtx, func(gtx C) D {
		callOp.Add(gtx.Ops)
		return dims
	})

	return dims
}

func (me *TextEditor) ToggleSearchBar(gtx C) {
	if me.searchbar.Visible() {
		me.searchbar.Hide(gtx)
	} else {
		me.searchbar.Show(gtx)
	}
}

func (me *TextEditor) LayoutStatus(gtx C, th *theme.Theme, srv *service.ServiceFacade) D {
	return me.status.Layout(gtx, th, me, srv)
}

func (me *TextEditor) update(gtx layout.Context, th *theme.Theme, settings *settings.EditorSettings) {
	if me.pendingExternalChange.Swap(false) {
		me.checkExternalChanges()
	}

	colorScheme := th.Get("codeColorScheme").(string)

	colorSchemeChanged := me.colorScheme == nil || me.colorScheme.Name != colorScheme
	if colorSchemeChanged {
		me.colorScheme = buildColorScheme(colorScheme)
		me.colorScheme.Background = gvcolor.MakeColor(th.Bg) // overwrite with global palette color.
		me.colorScheme.SelectColor = gvcolor.MakeColor(th.ContrastBg).MulAlpha(0x60)
		me.colorScheme.LineColor = me.colorScheme.Foreground.MulAlpha(0x20)
		me.colorScheme.LineNumberColor = me.colorScheme.Foreground.MulAlpha(0xb6)
		me.state.WithOptions(gvcode.WithColorScheme(*me.colorScheme))

		me.highlighter.Highlight(me.state)
		me.hoverTips.SetColorScheme(colorScheme)
	}

	if me.popup != nil {
		me.popup.TextSize = th.TextSize - 1
		me.popup.Theme = th.Theme
	}

	xScrollDist := me.xScroll.ScrollDistance()
	yScrollDist := me.yScroll.ScrollDistance()
	if xScrollDist != 0.0 || yScrollDist != 0.0 {
		me.state.Scroll(gtx, xScrollDist, yScrollDist)
	}

	me.handleEvents(gtx)

	me.overviewRuler.SetLines(me.state.Lines())
	me.updateDiagnostics()
	me.updateStatusBar(settings)

	// process events from hover tip window
	link, external := me.hoverTips.Update(gtx, th)
	if link != "" && me.OnOpenLink != nil {
		me.OnOpenLink(link, external)
	}

	if hunks := me.differ.PendingHunks(); hunks != nil {
		me.diffProvider.UpdateDiff(hunks)
		me.overviewRuler.UpdateDiffMarkers(hunks)
	}

	if tokens := me.highlighter.PendingTokens(); tokens != nil && len(*tokens) > 0 {
		me.state.SetSyntaxTokens(*tokens...)
	}

}

func (me *TextEditor) handleEvents(gtx layout.Context) {
	// handle editor events
	for {
		event, ok := me.state.Update(gtx)
		if !ok {
			break
		}

		switch evt := event.(type) {
		case gvcode.ChangeEvent:
			me.onTextChanged()
			me.highlighter.Highlight(me.state)
			me.searchbar.ReSearch()
			if me.lspClient != nil {
				me.lspClient.OnEditorUpdated(me.filename, me.state)
				me.state.OnTextEdit()
			}

			me.updateDiff()

		case gvcode.HoverEvent:
			if me.lspClient == nil {
				continue
			}
			me.hoverTips.OnHover(evt, me.queryDiagnosticsOnHover, me.queryDocOnHover)

		case gvcode.SelectEvent:
			me.hoverTips.Clear(gtx)

			if me.OnSelectChange != nil {
				start, end := me.state.Selection()
				if start != end {
					return
				}

				line, col := me.state.CaretPos()
				me.OnSelectChange(gvcode.Position{Line: line, Column: col})
			}
		}
	}

	// key and pointer handler
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: me.state, Name: "S", Required: key.ModShortcut},
			key.Filter{Focus: me.state, Name: "F", Required: key.ModShortcut},
			key.Filter{Focus: me.state, Name: "L", Required: key.ModShortcut},
			key.Filter{Focus: me.state, Name: "W", Required: key.ModShortcut},
			key.Filter{Focus: me.state, Name: key.NameEscape},
		)
		if !ok {
			break
		}

		switch event := e.(type) {
		case key.Event:
			if event.Modifiers == key.ModShortcut && event.State == key.Press {
				if event.Name == "S" {
					if me.state.Mode() != gvcode.ModeReadOnly {
						me.onTextChanged()
					}
				}
				if event.Name == "F" {
					me.searchbar.Show(gtx)
				}
				if event.Name == "L" {
					me.state.WithOptions(gvcode.ReadOnlyMode(me.state.Mode() != gvcode.ModeReadOnly))
				}
				if event.Name == "W" {
					me.state.WithOptions(gvcode.WrapLine(!me.wrapLine))
					me.wrapLine = !me.wrapLine
				}
			}

			if event.Name == key.NameEscape {
				me.searchbar.Hide(gtx)
			}
		}
	}

}

func (me *TextEditor) queryDocOnHover(pos gvcode.Position) (string, f32.Point) {
	result0, err := me.lspClient.Hover(context.Background(), me.filename, pos.Line, pos.Column)
	if err != nil {
		log.Printf("run lsp hover failed: %v", err)
		return "", f32.Point{}
	} else if result0 != nil {
		pixelPos := f32.Point{}

		if result0.Range != nil {
			_, pixelPos = me.state.ConvertPos(int(result0.Range.Start.Line), int(result0.Range.Start.Character))
		}

		return string(result0.Contents), pixelPos
	}

	return "", f32.Point{}
}

func (me *TextEditor) queryDiagnosticsOnHover(pos gvcode.Position) (string, f32.Point) {
	diagnostics := me.lspClient.Diagnostics(me.filename)
	if diagnostics == nil {
		return "", f32.Point{}
	}

	ppos := protocol.Position{Line: uint32(pos.Line), Character: uint32(pos.Column)}
	for _, diag := range diagnostics.Diagnostics {
		if protocol.ComparePosition(diag.Range.Start, ppos) <= 0 && protocol.ComparePosition(diag.Range.End, ppos) >= 0 {
			_, pixelPos := me.state.ConvertPos(int(diag.Range.Start.Line), int(diag.Range.Start.Character))

			return diag.Message, pixelPos
		}
	}

	return "", f32.Point{}
}

func (me *TextEditor) updateDiagnostics() {
	if me.lspClient == nil {
		return
	}

	// Add decorations to virsualize LSP diagnostics information.
	diagnostics := me.lspClient.Diagnostics(me.filename)
	if diagnostics != nil && diagnostics.Refreshed() {
		me.state.ClearDecorations(me.filename)
		me.diagnosticsDecos = me.diagnosticsDecos[:0]
		lineMarkers := make([]int, 0)
		for _, diag := range diagnostics.Diagnostics {
			start, _ := me.state.ConvertPos(int(diag.Range.Start.Line), int(diag.Range.Start.Character))
			end, _ := me.state.ConvertPos(int(diag.Range.End.Line), int(diag.Range.End.Character))
			me.diagnosticsDecos = append(me.diagnosticsDecos,
				decoration.Decoration{
					Source:   me.filename,
					Start:    start,
					End:      end,
					Squiggle: &decoration.Squiggle{Color: errHighlightColor},
				},
			)
			lineMarkers = append(lineMarkers, int(diag.Range.Start.Line))
		}

		me.overviewRuler.UpdateDiagnosticMarkers(lineMarkers...)

		if len(me.diagnosticsDecos) > 0 {
			err := me.state.AddDecorations(me.diagnosticsDecos...)
			if err != nil {
				log.Println("Failed to add decorations for linter error: ", err)
			}
		}
	}
}

func (me *TextEditor) updateStatusBar(settings *settings.EditorSettings) {
	me.ensureStatus()

	me.status.Pos.Y, me.status.Pos.X = me.state.CaretPos()
	me.status.SelectedChars = me.state.SelectionLen()
	if indent, size := me.state.TabStyle(); indent == gvcode.Spaces {
		me.status.Indentation = fmt.Sprintf("Spaces: %d", size)
	} else {
		me.status.Indentation = fmt.Sprintf("Tab: %d", size)
	}
	me.status.ReadOnly = me.state.Mode() == gvcode.ModeReadOnly

	if me.status.Encoding == "" {
		me.status.Encoding = fileEncoding(me.filename)
	}

	if me.status.EndOfLine == "" {
		lineEnding := me.state.DetectedLineEnding()
		if lineEnding == gvcode.CRLF {
			me.status.EndOfLine = "CRLF"
		} else {
			me.status.EndOfLine = "LF"
		}
	}

	me.status.Language = me.highlighter.LexerName()

	if strings.EqualFold(me.status.Language, "Typst") {
		me.status.CompilerVer = strings.TrimSpace(typst.CurrentVersion())
	}
}

func (me *TextEditor) onTextChanged() {
	if !me.autoSaver.IsRunning() {
		me.autoSaver.Start()
	}

	me.autoSaver.Update()
}

func (me *TextEditor) hasPendingChanges() bool {
	return me.autoSaver != nil && me.autoSaver.HasPendingChanges()
}

func (me *TextEditor) BindWorkspaceWatcher(srv *service.ServiceFacade) error {
	me.srv = srv

	if err := srv.WatchFile(me.filename); err != nil {
		return err
	}

	srv.EventBus().Subscribe(me, "editor.file.changed", `workspace\.file\.changed`, func(topic string, data interface{}) {
		evt, ok := data.(bus.FileChangedEvent)
		if !ok || filepath.Clean(evt.Path) != filepath.Clean(me.filename) {
			return
		}

		me.pendingExternalChange.Store(true)
	})

	// watch git branch switch
	if me.differ != nil {
		workspaceRoot := srv.CurrentProjectDir()
		if err := srv.WatchFile(filepath.Join(workspaceRoot, ".git", "HEAD")); err != nil {
			return err
		}

		if err := srv.WatchFile(filepath.Join(workspaceRoot, ".git", "index")); err != nil {
			return err
		}

		srv.EventBus().Subscribe(me, "editor.gitbranch.changed", `git\.branch\.changed`, func(topic string, data interface{}) {
			me.differ.ReloadBaseline(false)
		})

		srv.EventBus().Subscribe(me, "editor.git.staged", `git\.file\.staged`, func(topic string, data interface{}) {
			me.differ.ReloadBaseline(true)
		})
	}

	return nil
}

func (me *TextEditor) ensureStatus() *EditorStatus {
	if me.status == nil {
		me.status = &EditorStatus{}
	}

	return me.status
}

func (me *TextEditor) checkExternalChanges() {
	content, err := os.ReadFile(me.filename)
	if err != nil {
		me.ensureStatus().SaveErr = err
		return
	}

	newHash := calcDigest(content)
	if newHash == me.originalHash {
		me.ensureStatus().SaveErr = nil
		return
	}

	status := me.ensureStatus()
	if me.hasPendingChanges() {
		status.SaveErr = errors.New("file changed on disk while editor has unsaved changes")
		return
	}

	if err := me.reloadContent(content, newHash); err != nil {
		status.SaveErr = err
		return
	}

	status.SaveErr = nil
}

func (me *TextEditor) reloadContent(content []byte, hash string) error {
	start, end := me.state.Selection()
	me.state.SetText(string(content))

	textLen := me.state.Len()
	if start > textLen {
		start = textLen
	}
	if end > textLen {
		end = textLen
	}
	me.state.SetCaret(start, end)

	me.originalHash = hash
	me.highlighter.Highlight(me.state)
	if me.searchbar != nil {
		me.searchbar.ReSearch()
	}
	me.updateDiff()

	if me.lspClient != nil {
		me.lspClient.OnEditorUpdated(me.filename, me.state)
		me.lspClient.OnEditorSaved(me.filename)
	}

	return nil
}

// Let the LSP server(tinymist) detect the focused file.
func (me *TextEditor) FocusLsp() {
	if me.lspClient != nil {
		me.lspClient.OnEditorUpdated(me.filename, me.state)
	}
}

func (me *TextEditor) convertIndentation(kind gvcode.TabStyle, tabWidth int, text string) string {
	reader := bufio.NewReader(strings.NewReader(text))
	buf := &bytes.Buffer{}

	tabIndent := "\t"
	spaceIndent := strings.Repeat(" ", tabWidth)

	for {
		line, readErr := reader.ReadBytes('\n')
		if strings.TrimSpace(string(line)) == "" && readErr == nil {
			buf.Write(line)
			continue // Ignore empty lines
		}

		tabs := 0
		spaces := 0
		spaceCnt := 0
		for _, r := range line {
			if r == '\t' {
				tabs++
			} else if r == ' ' {
				spaceCnt++
				if spaceCnt == tabWidth {
					spaces++
					spaceCnt = 0
					continue
				}
			} else {
				// other chars
				break
			}
		}

		if tabs > 0 {
			if kind == gvcode.Tabs {
				buf.Write(line)
			} else {
				buf.WriteString(strings.Replace(string(line), tabIndent, spaceIndent, tabs))
			}
		} else if spaces > 0 {
			if kind == gvcode.Spaces {
				buf.Write(line)
			} else {
				buf.WriteString(strings.Replace(string(line), spaceIndent, tabIndent, spaces))
			}
		} else {
			buf.Write(line)
		}

		if readErr != nil {
			break
		}
	}

	return buf.String()

}

func (me *TextEditor) Close() error {
	me.autoSaver.Stop()
	me.highlighter.Close()
	if me.srv != nil {
		me.srv.EventBus().Unsubscribe(me)
		if err := me.srv.UnwatchFile(me.filename); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			log.Println("unwatch file failed: ", err)
		}

		err := me.srv.UnwatchFile(filepath.Join(me.srv.CurrentProjectDir(), ".git", "HEAD"))
		if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			log.Println("unwatch file failed: ", err)
		}

		err = me.srv.UnwatchFile(filepath.Join(me.srv.CurrentProjectDir(), ".git", "index"))
		if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			log.Println("unwatch file failed: ", err)
		}
	}

	if me.differ != nil {
		me.differ.Stop()
	}

	if me.lspClient != nil {
		me.lspClient.OnEditorClosed(me.filename)
	}
	return nil
}

func (te *TextEditor) SetupLsp(gtx layout.Context, client *lsp.Client) {
	te.lspClient = client

	// Setting up auto-completion.
	cm := &completion.DefaultCompletion{Editor: te.state}
	//cm.SetDelay(10 * time.Millisecond)
	completor := lsp.NewLspAutoCompletor(client, te.filename)
	if completor == nil {
		log.Println("failed to setup auto completor")
		return
	}

	te.popup = completion.NewCompletionPopup(te.state, cm)
	maxSize := gtx.Constraints.Max
	te.popup.Size = image.Point{
		X: min(gtx.Dp(unit.Dp(550)), maxSize.X),
		Y: min(gtx.Dp(unit.Dp(350)), maxSize.Y),
	}

	cm.AddCompletor(completor, te.popup)
	te.state.WithOptions(gvcode.WithAutoCompletion(cm))
	client.OnEditorUpdated(te.filename, te.state)
}

func (me *TextEditor) updateDiff() {
	if me.differ == nil {
		return
	}

	me.differ.Trigger(me.state)
}

func NewTextEditor(path string, showDiff bool, settings *settings.EditorSettings) (*TextEditor, error) {
	ed := &TextEditor{
		filename:     path,
		highlighter:  NewHighlighter(path),
		state:        &gvcode.Editor{},
		wrapLine:     false,
		diffProvider: providers.NewVCSDiffProvider(),
	}

	ed.state.WithOptions(
		gvcode.WrapLine(false),
		gvcode.WithGutterGap(unit.Dp(24)),
		gvcode.WithCornerRadius(unit.Dp(4)),
		gvcode.WithGutter(providers.NewLineNumberProvider()),
		gvcode.WithGutter(ed.diffProvider),
	)

	// Initialize overview ruler colors
	ed.overviewRuler.UseDefaultColors()
	ed.diffProvider.SetIndicatorWidth(unit.Dp(3))
	ed.hoverTips = newHoverTips(ed.state)
	if showDiff {
		ed.differ = NewGitDiff(ed.filename)
	}

	originalFile, err := os.OpenFile(path, os.O_RDWR, 0755)
	if err != nil {
		return nil, err
	}
	defer originalFile.Close()

	content, err := io.ReadAll(originalFile)
	if err != nil {
		return nil, err
	}

	if len(content) == 0 {
		ed.state.WithOptions(
			gvcode.WithSoftTab(settings.UseSoftTab == "true"),
			gvcode.WithTabWidth(settings.TabSize),
		)
	} // else guess by the editor

	ed.state.SetText(string(content))
	ed.originalHash = calcDigest(content)

	// Some file systems or file system watchers, like fswatch, might not
	// pick up changes if they are batched together or if the file is not
	// closed promptly after a write. For example, if you keep writing to
	// the file without closing it, some file system watchers may not report
	// the event until the file handle is closed or the write is flushed.
	ed.autoSaver = NewAutoSaver(time.Second*time.Duration(settings.AutoSaveInterval), func() error {
		file, err := os.OpenFile(path, os.O_RDWR, 0755)
		if err != nil {
			return err
		}

		defer file.Close()

		// detect changes before overwriting the file with editor buffer.
		content, err := io.ReadAll(file)
		if err != nil {
			return err
		}

		status := ed.ensureStatus()
		if newHash := calcDigest(content); newHash != ed.originalHash {
			err := errors.New("cannot save file as it has beed edited elsewhere")
			status.SaveErr = err
			return err
		} else {
			status.SaveErr = nil
		}

		file.Truncate(0)
		file.Seek(0, 0)

		editorContent := []byte(ed.state.PrepareForSave(ed.state.Text()))

		written, err := file.Write(editorContent)
		// written, err := io.Copy(file, ed.state.Text())
		if err != nil {
			return err
		}

		if written != len(editorContent) {
			return errors.New("write file error: partial write")
		}

		// update editor original hash
		ed.originalHash = calcDigest(editorContent)

		// On successful write, notify the LSP
		if ed.lspClient != nil {
			ed.lspClient.OnEditorSaved(ed.filename)
		}

		return nil
	})

	ed.updateDiff()

	return ed, nil
}

func calcDigest(content []byte) string {
	hasher := md5.New()
	hasher.Write(content)
	return hex.EncodeToString(hasher.Sum(nil))
}
