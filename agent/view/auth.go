package view

import (
	"context"
	"fmt"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/coder/acp-go-sdk"
	"github.com/oligo/gioview/theme"
	"looz.ws/typstify/i18n"
)

type AuthFunction func(ctx context.Context, methodID string) error

type AuthenticationView struct {
	agentInfo       acp.Implementation
	authMethods     []acp.AuthMethod
	authMethodClick []*widget.Clickable
	authFunc        AuthFunction
	authErr         error
	authSuccess     bool
}

func NewAuthenticationView(agentInfo acp.Implementation, authMethods []acp.AuthMethod, authFunc AuthFunction) *AuthenticationView {
	return &AuthenticationView{
		agentInfo:   agentInfo,
		authMethods: authMethods,
		authFunc:    authFunc,
	}
}

func (a *AuthenticationView) AuthErr() error {
	return a.authErr
}

func (a *AuthenticationView) Authenticated() bool {
	return a.authSuccess
}

func (a *AuthenticationView) Layout(gtx C, th *theme.Theme) D {
	for idx, clk := range a.authMethodClick {
		if clk.Clicked(gtx) {
			a.authErr = a.authFunc(context.Background(), methodID(a.authMethods[idx]))
			a.authSuccess = a.authErr == nil
		}
	}

	return layout.Center.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis: layout.Vertical,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				agentName := a.agentInfo.Name
				if a.agentInfo.Title != nil {
					agentName = *a.agentInfo.Title
				}
				title := material.H5(th.Theme, strings.ToTitle(agentName))
				title.Font.Weight = font.Bold
				return title.Layout(gtx)
			}),
			layout.Rigid(func(gtx C) D {
				lb := material.Label(th.Theme, th.TextSize, i18n.Translate("Please choose a method to authenticate the agent:"))
				return lb.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx C) D {
				options := make([]layout.FlexChild, 0, len(a.authMethods))
				for idx, method := range a.authMethods {
					if len(a.authMethodClick) <= idx {
						a.authMethodClick = append(a.authMethodClick, &widget.Clickable{})
					}
					idx := idx
					method := method
					options = append(options, layout.Rigid(func(gtx C) D {
						return material.Clickable(gtx, a.authMethodClick[idx], func(gtx C) D {
							return a.layoutAuthMethod(gtx, th, method)
						})
					}))
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(unit.Dp(4))}.Layout(gtx, options...)
			}),
		)
	})
}

func (a *AuthenticationView) layoutAuthMethod(gtx C, th *theme.Theme, m acp.AuthMethod) D {
	return widget.Border{
		Color:        th.Fg,
		Width:        unit.Dp(0.5),
		CornerRadius: unit.Dp(6),
	}.Layout(gtx, func(gtx C) D {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx C) D {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx C) D {
					id := methodID(m)
					if id == "" {
						return D{}
					}
					return a.fieldRow(gtx, th, i18n.Translate("ID"), id)
				}),
				layout.Rigid(func(gtx C) D {
					return a.fieldRow(gtx, th, i18n.Translate("Name"), methodName(m))
				}),

				layout.Rigid(func(gtx C) D {
					desc := methodDesc(m)
					if desc == "" {
						return D{}
					}
					return a.fieldRow(gtx, th, i18n.Translate("Description"), desc)
				}),
				layout.Rigid(func(gtx C) D {
					return a.layoutMethodDetail(gtx, th, m)
				}),
			)
		})
	})
}

func (a *AuthenticationView) layoutMethodDetail(gtx C, th *theme.Theme, m acp.AuthMethod) D {
	switch {
	case m.EnvVar != nil:
		return a.layoutEnvVarDetail(gtx, th, m.EnvVar)
	case m.Terminal != nil:
		return a.layoutTerminalDetail(gtx, th, m.Terminal)
	default:
		// Agent-managed auth — nothing extra to show.
		return D{}
	}
}

func (a *AuthenticationView) layoutEnvVarDetail(gtx C, th *theme.Theme, m *acp.AuthMethodEnvVarInline) D {
	var items []layout.FlexChild

	if m.Link != nil && *m.Link != "" {
		items = append(items, layout.Rigid(func(gtx C) D {
			return a.fieldRow(gtx, th, i18n.Translate("Link"), *m.Link)
		}))
	}

	if len(m.Vars) > 0 {
		items = append(items, layout.Rigid(func(gtx C) D {
			label := material.Label(th.Theme, th.TextSize, i18n.Translate("Required environment variables:"))
			label.Font.Weight = font.SemiBold
			return label.Layout(gtx)
		}))
		for _, v := range m.Vars {
			varLabel := v.Name
			if v.Label != nil && *v.Label != "" {
				varLabel = *v.Label + " (" + v.Name + ")"
			}
			items = append(items, layout.Rigid(func(gtx C) D {
				label := material.Label(th.Theme, th.TextSize, "  "+varLabel)
				return label.Layout(gtx)
			}))
		}
	}

	if len(items) == 0 {
		return D{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}

func (a *AuthenticationView) layoutTerminalDetail(gtx C, th *theme.Theme, m *acp.AuthMethodTerminalInline) D {
	var items []layout.FlexChild

	if len(m.Args) > 0 {
		items = append(items, layout.Rigid(func(gtx C) D {
			return a.fieldRow(gtx, th, i18n.Translate("Args"), strings.Join(m.Args, " "))
		}))
	}

	if len(m.Env) > 0 {
		items = append(items, layout.Rigid(func(gtx C) D {
			return a.fieldRow(gtx, th, i18n.Translate("Env"), fmt.Sprint(m.Env))
		}))
	}

	if len(items) == 0 {
		return D{}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}

func (a *AuthenticationView) fieldRow(gtx C, th *theme.Theme, label, value string) D {
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx C) D {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				l := material.Label(th.Theme, th.TextSize, label+":")
				l.Font.Weight = font.SemiBold
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				return material.Label(th.Theme, th.TextSize, value).Layout(gtx)
			}),
		)
	})
}

func methodName(m acp.AuthMethod) string {
	switch {
	case m.EnvVar != nil:
		return m.EnvVar.Name
	case m.Terminal != nil:
		return m.Terminal.Name
	case m.Agent != nil:
		return m.Agent.Name
	}
	return ""
}

func methodID(m acp.AuthMethod) string {
	switch {
	case m.EnvVar != nil:
		return m.EnvVar.Id
	case m.Terminal != nil:
		return m.Terminal.Id
	case m.Agent != nil:
		return m.Agent.Id
	}
	return ""
}

func methodDesc(m acp.AuthMethod) string {
	switch {
	case m.EnvVar != nil && m.EnvVar.Description != nil:
		return *m.EnvVar.Description
	case m.Terminal != nil && m.Terminal.Description != nil:
		return *m.Terminal.Description
	case m.Agent != nil && m.Agent.Description != nil:
		return *m.Agent.Description
	}
	return ""
}
