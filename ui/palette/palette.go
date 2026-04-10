package palette

import (
	"fmt"
	"slices"

	"github.com/oligo/gioview/misc"
	th "github.com/oligo/gioview/theme"
)

type UIPalette struct {
	th.Palette
	// chroma style name, see https://xyproto.github.io/splash/docs/ for the full list
	CodeColorScheme string
}

var themeMap = map[string]UIPalette{
	"Default Light": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0x383A42),
			Bg:            misc.HexColor(0xFAFAFA),
			ContrastFg:    misc.HexColor(0xFAFAFA),
			ContrastBg:    misc.HexColor(0x5A5A5A),
			Bg2:           misc.HexColor(0xF0F0F0),
			HoverAlpha:    30,
			SelectedAlpha: 50,
		},
		CodeColorScheme: "github",
	},

	"Default Dark": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0xABB2BF),
			Bg:            misc.HexColor(0x282C34),
			ContrastFg:    misc.HexColor(0x1F1B24),
			ContrastBg:    misc.HexColor(0x4B5263),
			Bg2:           misc.HexColor(0x21252B),
			HoverAlpha:    36,
			SelectedAlpha: 48,
		},
		CodeColorScheme: "doom-one",
	},

	"Solarized Light": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0x657b83),
			Bg:            misc.HexColor(0xfdf6e3),
			ContrastFg:    misc.HexColor(0xfdf6e3),
			ContrastBg:    misc.HexColor(0xCB4B16),
			Bg2:           misc.HexColor(0xeee8d5),
			HoverAlpha:    30,
			SelectedAlpha: 40,
		},
		CodeColorScheme: "solarized-light",
	},

	"Solarized Dark": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0x839496), // Softer foreground
			Bg:            misc.HexColor(0x002b36), // Deep navy
			ContrastFg:    misc.HexColor(0x073642), // Slightly lighter
			ContrastBg:    misc.HexColor(0x268BD2), // Solarized blue for accent
			Bg2:           misc.HexColor(0x073642), // Lighter navy for surfaces
			HoverAlpha:    25,
			SelectedAlpha: 40,
		},
		CodeColorScheme: "solarized-dark",
	},

	// Nord - Nordic frost palette
	"Nord Light": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0x2E3440),
			Bg:            misc.HexColor(0xECEFF4),
			ContrastFg:    misc.HexColor(0xD8DEE9),
			ContrastBg:    misc.HexColor(0x5E81AC),
			Bg2:           misc.HexColor(0xE5E9F0),
			HoverAlpha:    30,
			SelectedAlpha: 50,
		},
		CodeColorScheme: "monokailight",
	},

	"Nord Dark": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0xD8DEE9),
			Bg:            misc.HexColor(0x2E3440),
			ContrastFg:    misc.HexColor(0x2E3440),
			ContrastBg:    misc.HexColor(0x4C566A),
			Bg2:           misc.HexColor(0x3B4252),
			HoverAlpha:    25,
			SelectedAlpha: 40,
		},
		CodeColorScheme: "nord",
	},

	// Dracula - Vibrant purple/pink
	"Dracula": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0xF8F8F2),
			Bg:            misc.HexColor(0x282A36),
			ContrastFg:    misc.HexColor(0x282A36),
			ContrastBg:    misc.HexColor(0xBD93F9),
			Bg2:           misc.HexColor(0x343746),
			HoverAlpha:    35,
			SelectedAlpha: 55,
		},
		CodeColorScheme: "dracula",
	},

	// Gruvbox - Retro warm palette
	"Gruvbox Light": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0x3C3C3C),
			Bg:            misc.HexColor(0xFBF1C7),
			ContrastFg:    misc.HexColor(0xF9F5D7),
			ContrastBg:    misc.HexColor(0x9D0000),
			Bg2:           misc.HexColor(0xEBDBB2),
			HoverAlpha:    35,
			SelectedAlpha: 55,
		},
		CodeColorScheme: "gruvbox-light",
	},

	"Gruvbox Dark": {
		Palette: th.Palette{
			Fg:            misc.HexColor(0xEBDBB2),
			Bg:            misc.HexColor(0x282828),
			ContrastFg:    misc.HexColor(0x1D2021),
			ContrastBg:    misc.HexColor(0xCC241D),
			Bg2:           misc.HexColor(0x32302F),
			HoverAlpha:    30,
			SelectedAlpha: 45,
		},
		CodeColorScheme: "gruvbox-dark",
	},
}

func ThemeNames() []string {
	var names []string
	for k := range themeMap {
		names = append(names, k)
	}

	slices.SortFunc(names, func(a, b string) int {
		if a == "Default Light" {
			return -1
		}
		if b == "Default Light" {
			return 1
		}

		if a >= b {
			return 1
		} else {
			return -1
		}
	})

	return names
}

func ThemeConfig(themeName string) (UIPalette, error) {
	p, ok := themeMap[themeName]
	if !ok {
		return UIPalette{}, fmt.Errorf("no theme found for name: %s", themeName)
	}

	return p, nil
}
