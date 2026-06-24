package settings

import (
	"context"
	"image"
	"image/color"
	"log"
	"os/exec"
	"sort"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/misc"
	gvwidget "github.com/oligo/gioview/widget"

	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service/settings"
	"looz.ws/typstify/widgets"

	"github.com/oligo/gioview/theme"
)

type AgentView struct {
	setting *settings.AcpAgentSettings

	agentDropdown *widgets.Dropdown
	fetchBtn      widget.Clickable
	registryErr   error
	registryDone  bool

	customNameField gvwidget.TextField
	customCmdField  gvwidget.TextField

	repoLink    widget.Clickable
	binaryLinks map[string]*widget.Clickable

	npxReady   bool
	uvxReady   bool
	checkedEnv bool

	isInitialized bool
	lastErr       error
}

func (v *AgentView) Title() string {
	return i18n.Translate("Agent")
}

func (v *AgentView) checkRuntimes() {
	if v.checkedEnv {
		return
	}
	v.checkedEnv = true
	_, err := exec.LookPath("npx")
	v.npxReady = err == nil
	_, err = exec.LookPath("uvx")
	v.uvxReady = err == nil
}

// resolveStoredCommand converts a registry entry to cmd/args strings and saves.
func (v *AgentView) resolveAndSave(entry *settings.AgentEntry) {
	v.setting.AgentID = entry.ID
	v.setting.AgentName = entry.Name
	switch entry.DistKind() {
	case "npx":
		v.setting.Cmd = "npx"
		v.setting.Args = "-y " + entry.Distribution.Npx.Package + " " + joinArgs(entry.Distribution.Npx.Args)
	case "uvx":
		v.setting.Cmd = "uvx"
		v.setting.Args = entry.Distribution.Uvx.Package + " " + joinArgs(entry.Distribution.Uvx.Args)
	default:
		// binary — use the first available binary's cmd and args
		for _, bin := range entry.Distribution.Binary {
			v.setting.Cmd = bin.Cmd
			v.setting.Args = joinArgs(bin.Args)
			break
		}
		if v.setting.Cmd == "" {
			v.setting.Cmd = ""
			v.setting.Args = ""
		}
	}
	v.lastErr = v.setting.Save()
}

func linkLabel(gtx C, th *theme.Theme, text string) D {
	l := material.Label(th.Theme, th.TextSize, text)
	l.Color = th.ContrastBg
	dims := l.Layout(gtx)
	// underline
	offStack := op.Offset(image.Point{Y: dims.Size.Y}).Push(gtx.Ops)
	lineH := gtx.Dp(unit.Dp(1))
	rect := clip.Rect{Max: image.Point{X: dims.Size.X, Y: lineH}}
	paint.FillShape(gtx.Ops, th.ContrastBg, rect.Op())
	offStack.Pop()
	return dims
}

func joinArgs(args []string) string {
	return strings.Join(args, " ")
}

func (v *AgentView) Layout(gtx C, th *theme.Theme) D {
	v.checkRuntimes()

	if !v.isInitialized {
		v.customNameField.SingleLine = true
		v.customNameField.SetText(v.setting.AgentName)
		v.customCmdField.SingleLine = true
		if v.setting.Args != "" {
			v.customCmdField.SetText(v.setting.Cmd + " " + v.setting.Args)
		} else {
			v.customCmdField.SetText(v.setting.Cmd)
		}
		v.isInitialized = true
	}

	if v.agentDropdown != nil {
		if v.agentDropdown.Update(gtx) {
			id := v.agentDropdown.Value()
			if id != v.setting.AgentID {
				if entry := settings.LookupAgent(id); entry != nil {
					v.resolveAndSave(entry)
					v.customNameField.SetText(v.setting.AgentName)
					v.customCmdField.SetText(v.setting.Cmd + " " + v.setting.Args)
				}
			}
		}
	}
	if v.customNameField.Submitted() {
		v.setting.AgentID = ""
		v.setting.AgentName = v.customNameField.Text()
		v.lastErr = v.setting.Save()
	}
	if v.customCmdField.Submitted() {
		parts := strings.Fields(v.customCmdField.Text())
		if len(parts) > 0 {
			v.setting.AgentID = ""
			v.setting.Cmd = parts[0]
			v.setting.Args = strings.Join(parts[1:], " ")
			v.lastErr = v.setting.Save()
		}
	}

	if v.fetchBtn.Clicked(gtx) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_, err := settings.FetchAgentRegistry(ctx)
			if err != nil {
				log.Printf("fetch agent registry: %v", err)
				v.registryErr = err
			} else {
				v.registryErr = nil
				v.registryDone = true
			}
		}()
	}

	return layout.Inset{Top: unit.Dp(20)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if v.lastErr != nil {
					label := material.Label(th.Theme, th.TextSize, v.lastErr.Error())
					label.Color = color.NRGBA{R: 0xc0, G: 0x40, B: 0x40, A: 0xff}
					return label.Layout(gtx)
				}
				return D{}
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx C) D {
				return v.layoutDropdown(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx C) D {
				return v.layoutSelectedDetail(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx C) D {
				return v.layoutCommandForm(gtx, th)
			}),
		)
	})
}

func (v *AgentView) ensureDropdown() {
	if v.agentDropdown != nil {
		return
	}

	reg, _ := settings.FetchAgentRegistry(context.Background())

	options := make(map[string]any, 50)
	if reg != nil {
		for i := range reg.Agents {
			a := &reg.Agents[i]
			options[a.ID] = a.Name
		}
	}

	v.agentDropdown = widgets.NewDropDown(options)
	if v.setting.AgentID != "" {
		v.agentDropdown.SetSelected(v.setting.AgentID)
	}
	v.agentDropdown.MaxHeight = unit.Dp(200)
}

func (v *AgentView) layoutDropdown(gtx C, th *theme.Theme) D {
	v.ensureDropdown()

	return settingItem{}.Layout(gtx, th,
		i18n.Translate("Agent"),
		i18n.Translate("Select an agent from the registry. Custom configuration below takes precedence."),
		func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(300))
					return v.agentDropdown.Layout(gtx, th)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx C) D {
					btn := material.Button(th.Theme, &v.fetchBtn, i18n.Translate("Refresh"))
					return btn.Layout(gtx)
				}),
			)
		},
	)
}

func (v *AgentView) layoutSelectedDetail(gtx C, th *theme.Theme) D {
	entry := settings.LookupAgent(v.setting.AgentID)
	if entry == nil {
		return D{}
	}

	authors := strings.Join(entry.Authors, ", ")

	field := func(label, value string) layout.FlexChild {
		if value == "" {
			return layout.Rigid(func(gtx C) D { return D{} })
		}
		return layout.Rigid(func(gtx C) D {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					l := material.Label(th.Theme, th.TextSize, label)
					l.Font.Weight = font.SemiBold
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx C) D {
					l := material.Label(th.Theme, th.TextSize, value)
					l.Color = th.Fg
					return l.Layout(gtx)
				}),
			)
		})
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			l := material.H6(th.Theme, entry.Name)
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			l := material.Label(th.Theme, th.TextSize, entry.Description)
			return l.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		field(i18n.Translate("ID:"), entry.ID),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		field(i18n.Translate("Version:"), entry.Version),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		field(i18n.Translate("License:"), entry.License),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		field(i18n.Translate("Authors:"), authors),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx C) D {
			if entry.Repository == "" {
				return D{}
			}
			if v.repoLink.Clicked(gtx) {
				giohyperlink.Open(entry.Repository)
			}
			return v.repoLink.Layout(gtx, func(gtx C) D {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						l := material.Label(th.Theme, th.TextSize, i18n.Translate("Repository:"))
						l.Font.Weight = font.SemiBold
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx C) D {
						return linkLabel(gtx, th, entry.Repository)
					}),
				)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx C) D {
			return v.layoutInstallGuide(gtx, th, entry)
		}),
	)
}

func (v *AgentView) layoutInstallGuide(gtx C, th *theme.Theme, entry *settings.AgentEntry) D {
	kind := entry.DistKind()

	switch kind {
	case "npx":
		ok := v.npxReady
		status := i18n.Translate("Ready \u2014 npx is available on your system.")
		statusColor := color.NRGBA{R: 0x40, G: 0xa0, B: 0x40, A: 0xff}
		fallback := ""
		if !ok {
			status = i18n.Translate("Requires Node.js. Install from https://nodejs.org, then restart Typstify.")
			statusColor = color.NRGBA{R: 0xc0, G: 0x40, B: 0x40, A: 0xff}
		} else {
			pkg := entry.Distribution.Npx
			if pkg != nil && pkg.Package != "" {
				fallback = i18n.Translate("If the agent fails to start, try installing manually: npm install -g %s", pkg.Package)
			}
		}
		return settingItem{}.Layout(gtx, th,
			i18n.Translate("Installation (npx)"),
			"",
			func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						label := material.Label(th.Theme, th.TextSize, status)
						label.Color = statusColor
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						if fallback == "" {
							return D{}
						}
						label := material.Label(th.Theme, th.TextSize*0.85, fallback)
						label.Color = misc.WithAlpha(th.Fg, 0xb0)
						return label.Layout(gtx)
					}),
				)
			},
		)

	case "uvx":
		ok := v.uvxReady
		status := i18n.Translate("Ready \u2014 uvx is available on your system.")
		statusColor := color.NRGBA{R: 0x40, G: 0xa0, B: 0x40, A: 0xff}
		fallback := ""
		if !ok {
			status = i18n.Translate("Requires uvx. Install via `pip install uv` or https://docs.astral.sh/uv, then restart Typstify.")
			statusColor = color.NRGBA{R: 0xc0, G: 0x40, B: 0x40, A: 0xff}
		} else {
			pkg := entry.Distribution.Uvx
			if pkg != nil && pkg.Package != "" {
				fallback = i18n.Translate("If the agent fails to start, try installing manually: uv pip install %s", pkg.Package)
			}
		}
		return settingItem{}.Layout(gtx, th,
			i18n.Translate("Installation (uvx)"),
			"",
			func(gtx C) D {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						label := material.Label(th.Theme, th.TextSize, status)
						label.Color = statusColor
						return label.Layout(gtx)
					}),
					layout.Rigid(func(gtx C) D {
						if fallback == "" {
							return D{}
						}
						label := material.Label(th.Theme, th.TextSize*0.85, fallback)
						label.Color = misc.WithAlpha(th.Fg, 0xb0)
						return label.Layout(gtx)
					}),
				)
			},
		)

	case "binary":
		if len(entry.Distribution.Binary) > 0 {
			return settingItem{}.Layout(gtx, th,
				i18n.Translate("Installation (Binary)"),
				i18n.Translate("Download the archive for your platform, extract it, and ensure the binary is on your PATH."),
				func(gtx C) D {
					var items []layout.FlexChild
					platforms := make([]string, 0, len(entry.Distribution.Binary))
					for p := range entry.Distribution.Binary {
						platforms = append(platforms, p)
					}
					sort.Strings(platforms)
					for _, platform := range platforms {
						url := entry.Distribution.Binary[platform].Archive
						if v.binaryLinks == nil {
							v.binaryLinks = make(map[string]*widget.Clickable)
						}
						if v.binaryLinks[url] == nil {
							v.binaryLinks[url] = &widget.Clickable{}
						}
						link := v.binaryLinks[url]
						if link.Clicked(gtx) {
							giohyperlink.Open(url)
						}
						items = append(items, layout.Rigid(func(gtx C) D {
							return link.Layout(gtx, func(gtx C) D {
								return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										label := material.Label(th.Theme, th.TextSize, platform)
										label.Font.Weight = font.SemiBold
										return label.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx C) D {
										return linkLabel(gtx, th, url)
									}),
								)
							})
						}))
						items = append(items, layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout))
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
				},
			)
		}
		fallthrough

	default:
		return settingItem{}.Layout(gtx, th,
			i18n.Translate("Installation"),
			i18n.Translate("This agent is not directly available for your platform. Visit the ACP registry for more details."),
			func(gtx C) D {
				return linkLabel(gtx, th, i18n.Translate("See https://agentclientprotocol.com/get-started/registry"))
			},
		)
	}
}

func (v *AgentView) layoutCommandForm(gtx C, th *theme.Theme) D {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return settingItem{}.Layout(gtx, th,
				i18n.Translate("Agent Name"),
				i18n.Translate("Select an agent above or type a name directly."),
				func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(300))
					return v.customNameField.Layout(gtx, th, i18n.Translate("Name"))
				},
			)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
		layout.Rigid(func(gtx C) D {
			return settingItem{}.Layout(gtx, th,
				i18n.Translate("Command"),
				i18n.Translate("Select an agent above or edit directly, e.g. npx -y @scope/package"),
				func(gtx C) D {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(300))
					return v.customCmdField.Layout(gtx, th, i18n.Translate("npx -y @scope/package"))
				},
			)
		}),
	)
}
