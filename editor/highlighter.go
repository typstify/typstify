package editor

import (
	"image/color"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/oligo/gvcode"
	gvcolor "github.com/oligo/gvcode/color"
	"github.com/oligo/gvcode/textstyle/syntax"
	"github.com/saintfish/chardet"
)

const highlightDebounce = 50 * time.Millisecond

var camelCaseRe = regexp.MustCompile(`([A-Z]+)`)

type highlightResult struct {
	seq    uint64
	tokens *[]syntax.Token
}

type highlightJob struct {
	seq    uint64
	editor *gvcode.Editor
}

type Highlighter struct {
	lexer         chroma.Lexer
	seq           atomic.Uint64
	pendingResult atomic.Pointer[highlightResult]
	jobs          chan highlightJob
	done          chan struct{}
	closed        atomic.Bool
}

func NewHighlighter(filename string) *Highlighter {
	fileExt := filepath.Ext(filename)
	lexer := lexers.Get(fileExt)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	lexer = chroma.Coalesce(lexer)

	h := &Highlighter{
		lexer: lexer,
		jobs:  make(chan highlightJob, 1),
		done:  make(chan struct{}),
	}

	h.startWorker()
	return h
}

func chromaTokenType2Scope(t chroma.TokenType) syntax.StyleScope {
	str := camelCaseRe.ReplaceAllString(t.String(), `.$1`)
	str = strings.ToLower(strings.Trim(str, "."))

	return syntax.StyleScope(str)
}

func convertChromaColor(c chroma.Colour) gvcolor.Color {
	if !c.IsSet() {
		return gvcolor.Color{}
	}

	return gvcolor.MakeColor(color.NRGBA{
		R: c.Red(),
		G: c.Green(),
		B: c.Blue(),
		A: 0xff,
	})
}

// Highlight debounces and tokenizes the editor content asynchronously.
// Results are picked up via PendingTokens on the next frame.
func (h *Highlighter) Highlight(editor *gvcode.Editor) {
	if h.closed.Load() {
		return
	}
	seq := h.seq.Add(1)
	job := highlightJob{seq: seq, editor: editor}
	h.enqueueLatest(job)
}

// enqueueLatest keeps only the newest job in the 1-slot queue without blocking the caller.
func (h *Highlighter) enqueueLatest(job highlightJob) {
	for {
		select {
		case <-h.done:
			return
		case h.jobs <- job:
			return
		default:
			// Queue full: drop stale job and retry.
			select {
			case <-h.jobs:
			default:
			}
		}
	}
}

func (h *Highlighter) startWorker() {
	go func() {
		var (
			timer      *time.Timer
			timerCh    <-chan time.Time
			latest     highlightJob
			haveLatest bool
		)

		for {
			select {
			case <-h.done:
				if timer != nil {
					timer.Stop()
				}
				return
			case job := <-h.jobs:
				latest = job
				haveLatest = true
				if timer == nil {
					timer = time.NewTimer(highlightDebounce)
					timerCh = timer.C
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(highlightDebounce)
				}
			case <-timerCh:
				if !haveLatest {
					continue
				}
				haveLatest = false
				if latest.seq != h.seq.Load() {
					continue
				}
				tokens := h.tokenize(latest.editor)
				if latest.seq != h.seq.Load() {
					continue
				}
				h.pendingResult.Store(&highlightResult{seq: latest.seq, tokens: &tokens})
			}
		}
	}()
}

func (h *Highlighter) Close() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	close(h.done)
}

func (h *Highlighter) tokenize(editor *gvcode.Editor) []syntax.Token {
	if editor == nil {
		return nil
	}
	reader := editor.GetReader()
	reader.Seek(0, io.SeekStart)
	source, err := io.ReadAll(reader)
	if err != nil {
		return nil
	}
	iterator, err := h.lexer.Tokenise(nil, string(source))
	if err != nil {
		return nil
	}

	newTokens := make([]syntax.Token, 0)
	offset := 0
	for _, token := range iterator.Tokens() {
		tokenLen := utf8.RuneCountInString(token.Value)
		if tokenLen <= 0 {
			continue
		}

		textStyle := syntax.Token{
			Start: offset,
			End:   offset + tokenLen,
			Scope: chromaTokenType2Scope(token.Type),
		}

		newTokens = append(newTokens, textStyle)
		offset = textStyle.End
	}

	return newTokens
}

func (h *Highlighter) PendingTokens() *[]syntax.Token {
	result := h.pendingResult.Swap(nil)
	if result == nil || result.seq != h.seq.Load() {
		return nil // no result, or stale result from before a newer edit
	}
	return result.tokens
}

func (h *Highlighter) LexerName() string {
	if h.lexer == nil || h.lexer.Config().Name == "fallback" {
		return "Plaintext"
	}

	return h.lexer.Config().Name
}

func fileEncoding(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return "-"
	}

	defer file.Close()

	var buf = make([]byte, 1*1024*1024)
	n, _ := file.Read(buf)
	buf = buf[:n]

	// Strategy A: High-Confidence Check
	// If the entire sample is valid UTF-8, assume UTF-8 with high confidence,
	// even if chardet might suggest ISO-8859-1 due to low character set diversity.
	if utf8.Valid(buf) {
		return "UTF-8"
	}

	// Strategy B: Fall back to heuristic detection
	detector := chardet.NewTextDetector()
	result, err := detector.DetectBest(buf)
	if err != nil {
		return "-"
	}

	return result.Charset
}

func buildColorScheme(schemeName string) *syntax.ColorScheme {
	style := styles.Get(schemeName)
	if style == nil {
		style = styles.Fallback
	}

	scheme := syntax.ColorScheme{Name: schemeName}

	bgType := style.Get(chroma.Background)
	fg := convertChromaColor(bgType.Colour)
	bg := convertChromaColor(bgType.Background)
	if fg.IsSet() {
		scheme.Foreground = fg
	} else {
		fg, _ = gvcolor.Hex2Color("#000000ff")
		scheme.Foreground = fg
	}
	if bg.IsSet() {
		scheme.Background = bg
	}

	for _, t := range style.Types() {
		style := style.Get(t)
		textStyle := syntax.TextStyle(0)
		if style.Bold == chroma.Yes {
			textStyle |= syntax.Bold
		}
		if style.Italic == chroma.Yes {
			textStyle |= syntax.Italic
		}
		if style.Border.IsSet() {
			textStyle |= syntax.Border
		}
		if style.Underline == chroma.Yes {
			textStyle |= syntax.Underline
		}

		var tokenFg, tokenBg gvcolor.Color
		if style.Colour.IsSet() {
			c := color.NRGBA{
				R: style.Colour.Red(),
				G: style.Colour.Green(),
				B: style.Colour.Blue(),
				A: 0xff,
			}
			tokenFg = gvcolor.MakeColor(c)
		} else {
			tokenFg = fg
		}

		// if style.Background.IsSet() {
		// 	c := color.NRGBA{
		// 		R: style.Background.Red(),
		// 		G: style.Background.Green(),
		// 		B: style.Background.Blue(),
		// 		A: 0xff,
		// 	}
		// 	tokenBg = gvcolor.MakeColor(c)
		// }

		scheme.AddStyle(chromaTokenType2Scope(t), textStyle, tokenFg, tokenBg)
	}

	return &scheme
}

func byteToRuneIndex(s string, byteIndex int) int {
	return utf8.RuneCountInString(s[:byteIndex])
}
