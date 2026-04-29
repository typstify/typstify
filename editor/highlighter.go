package editor

import (
	"image/color"
	"io"
	"log"
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
	"looz.ws/typstify/utils"
)

var camelCaseRe = regexp.MustCompile(`([A-Z]+)`)

var (
	defaultHighlightDebounce    = time.Millisecond * 150
	defaultMaxHighlightInterval = time.Millisecond * 250
)

type Highlighter struct {
	lexer         chroma.Lexer
	pendingResult atomic.Pointer[pendingHighlight]
	debouncer     *utils.Debouncer
}

func NewHighlighter(filename string) *Highlighter {
	fileExt := filepath.Ext(filename)
	lexer := lexers.Get(fileExt)
	if lexer == nil {
		lexer = lexers.Fallback
	}

	lexer = chroma.Coalesce(lexer)

	return &Highlighter{
		lexer: lexer,
		debouncer: &utils.Debouncer{
			Debounce:    defaultHighlightDebounce,
			MaxInterval: defaultMaxHighlightInterval,
		},
	}

}

type pendingHighlight struct {
	tokens    []syntax.Token
	sourceLen int
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

// Highlight debounces and tokenizes the editor content.
// Results are picked up via PendingTokens on the next frame.
func (h *Highlighter) Highlight(editor *gvcode.Editor) {
	if editor == nil {
		return
	}
	reader := editor.GetReader()
	reader.Seek(0, io.SeekStart)

	h.debouncer.Run(func() {
		h.tokenize(reader)
	})

}

func (h *Highlighter) Close() {
	h.debouncer.Stop()
}

func (h *Highlighter) tokenize(reader io.Reader) {
	source, err := io.ReadAll(reader)
	if err != nil {
		log.Println("read from editor error: ", err)
		return
	}

	iterator, err := h.lexer.Tokenise(nil, string(source))
	if err != nil {
		return
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

	// newTokens will never be nil, so we can distinct no changes and not computed.
	h.pendingResult.Store(&pendingHighlight{
		tokens:    newTokens,
		sourceLen: len(source),
	})
}

func (h *Highlighter) PendingTokens(textLen int) []syntax.Token {
	lastResult := h.pendingResult.Swap(nil)
	if lastResult == nil {
		return nil
	}
	if textLen != lastResult.sourceLen {
		return nil // text changed since tokenization, discard stale tokens
	}
	return lastResult.tokens
}

func (h *Highlighter) Language() string {
	if h.lexer == nil {
		return "unknown"
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

		scope := chromaTokenType2Scope(t)

		// Don't let chroma's TextWhitespace color override the default text.
		//
		// In rapid typing, before the new tokens are applied to editor, there
		// will be a one character offset between the editor content and the tokens,
		// and gvcode editor will shifts the exiting tokens by the amount of the newly
		// inserted text. The previous token tries to absorb the new added text,
		// so if are are typing after a white space token and the white space character happens
		// to have a different color, the newly added text will have the same color as white space.
		// Here we set the white space color to a default text color, so as to make the text looks stable.
		if scope == "text.whitespace" {
			tokenFg = gvcolor.Color{}
		}

		scheme.AddStyle(scope, textStyle, tokenFg, tokenBg)
	}

	return &scheme
}

func byteToRuneIndex(s string, byteIndex int) int {
	return utf8.RuneCountInString(s[:byteIndex])
}
