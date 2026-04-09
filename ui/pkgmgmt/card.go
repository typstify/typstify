package pkgmgmt

import (
	"fmt"
	stdimg "image"
	"io"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/gesture"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/inkeliz/giohyperlink"
	"github.com/oligo/gioview/image"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	gvwiget "github.com/oligo/gioview/widget"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/typst/pkg"
)

type PkgCard struct {
	pkgInfo     pkg.TypstPkg
	thumb       *PkgThumb
	copyBtn     widget.Clickable
	downloadBtn widget.Clickable
	docBtn      widget.Clickable

	onDownloadClicked func(pkgInfo *pkg.TypstPkg)
}

type PkgThumb struct {
	thumb     *image.ImageSource
	rawImgLoc string
	click     gesture.Click
	hovering  bool
	onClick   func(imgPath string)
}

func newPkgCard(pkg pkg.TypstPkg, onDownloadClicked func(pkgInfo *pkg.TypstPkg)) *PkgCard {
	return &PkgCard{
		pkgInfo:           pkg,
		onDownloadClicked: onDownloadClicked,
	}
}

func (c *PkgCard) update(gtx C) {
	if c.copyBtn.Clicked(gtx) {
		c.copyImportPath(gtx)
	}

	if c.downloadBtn.Clicked(gtx) && c.onDownloadClicked != nil {
		c.onDownloadClicked(&c.pkgInfo)
	}

	if c.docBtn.Clicked(gtx) {
		c.openPkgDocPage()
	}
}

func (c *PkgCard) Layout(gtx C, th *theme.Theme) D {
	c.update(gtx)

	macro := op.Record(gtx.Ops)
	dims := c.layout(gtx, th)
	callOp := macro.Stop()

	defer clip.UniformRRect(stdimg.Rectangle{Max: dims.Size}, 12).Push(gtx.Ops).Pop()
	paint.ColorOp{Color: misc.WithAlpha(th.Fg, 0x10)}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	callOp.Add(gtx.Ops)

	return dims

}

func (c *PkgCard) layout(gtx C, th *theme.Theme) D {
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				if c.thumb == nil {
					return D{}
				}
				return c.thumb.Layout(gtx, th)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),

			layout.Rigid(func(gtx C) D {
				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Flex{
							Axis:      layout.Horizontal,
							Alignment: layout.Middle,
							Spacing:   layout.SpaceBetween,
						}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								return layout.Flex{
									Alignment: layout.Middle,
									Gap:       gtx.Dp(unit.Dp(4)),
								}.Layout(gtx,
									layout.Rigid(func(gtx C) D {
										fillColor := th.Fg
										if c.pkgInfo.IsTemplate {
											fillColor = th.ContrastBg
										}
										return packageIcon.Layout(gtx, fillColor, th.TextSize*24.0/16.0)
									}),
									layout.Rigid(func(gtx C) D {
										return material.H5(th.Theme, c.pkgInfo.Name).Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
							layout.Rigid(func(gtx C) D {
								h := material.Label(th.Theme, th.TextSize, c.pkgInfo.LatestVersion)
								h.Color = misc.WithAlpha(th.Fg, 0xb6)
								return h.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
							layout.Flexed(1, func(gtx C) D {
								indicator := ""
								if c.pkgInfo.IsCached {
									indicator = i18n.Translate("Cached")
								}

								if indicator == "" {
									return D{}
								}

								return gvwiget.Tag{
									Text:       indicator,
									TextSize:   th.TextSize * 0.7,
									TextColor:  th.ContrastFg,
									Background: th.ContrastBg,
									Radius:     unit.Dp(8),
									Inset:      layout.UniformInset(unit.Dp(4)),
									Variant:    gvwiget.Solid,
								}.Layout(gtx, th)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(th.TextSize)}.Layout),
					layout.Rigid(func(gtx C) D {
						lb := material.Label(th.Theme, th.TextSize, c.pkgInfo.Description)
						lb.Color = misc.WithAlpha(th.Fg, 0xf6)
						return lb.Layout(gtx)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

					layout.Rigid(func(gtx C) D {
						return layoutLabel(gtx, th, "Author", strings.Join(c.pkgInfo.Authors, ","))
					}),
					layout.Rigid(func(gtx C) D {
						if c.pkgInfo.CreatedAt.IsZero() {
							return D{}
						}
						return layoutLabel(gtx, th, "Last Updated", c.pkgInfo.PublishedAt.Format(time.DateOnly))
					}),
					layout.Rigid(func(gtx C) D {
						return layoutLabel(gtx, th, "License", c.pkgInfo.License)
					}),
					layout.Rigid(func(gtx C) D {
						return layoutLabel(gtx, th, "Category", strings.Join(c.pkgInfo.Categories, ","))
					}),
					// layout.Rigid(func(gtx C) D {
					// 	return layoutLabel(gtx, th, "Minimum Typst version", c.pkgInfo.LatestVersion)
					// }),
					// layout.Rigid(func(gtx C) D {
					// 	return layoutLabel(gtx, th, "Repository", c.pkgInfo.)
					// }),
					layout.Rigid(func(gtx C) D {
						return layoutLabel(gtx, th, "Namespace", c.pkgInfo.Namespace)
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx C) D {
						// gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Flex{
							Axis:      layout.Horizontal,
							Alignment: layout.Middle,
							Spacing:   layout.SpaceBetween,
						}.Layout(gtx,
							layout.Rigid(func(gtx C) D {
								btn := material.Button(th.Theme, &c.docBtn, "Read the docs")
								btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(4), Right: unit.Dp(4)}
								return btn.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

							layout.Rigid(func(gtx C) D {
								btn := material.Button(th.Theme, &c.copyBtn, "Copy import path")
								btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(4), Right: unit.Dp(4)}
								return btn.Layout(gtx)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),

							layout.Rigid(func(gtx C) D {

								btn := material.Button(th.Theme, &c.downloadBtn, "Download")
								btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(4), Right: unit.Dp(4)}
								return btn.Layout(gtx)
							}),
						)
					}),
				)
			}),
		)
	})
}

func (c *PkgCard) copyImportPath(gtx C) {
	//instruction := fmt.Sprintf("#import \"@%s/%s:%s\"", c.pkgInfo.Namespace, c.pkgInfo.Name, c.pkgInfo.Version)
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(c.pkgInfo.ImportPath()))})
}

func (c *PkgCard) openPkgDocPage() {
	if c.pkgInfo.IsCached {
		return
	}
	giohyperlink.Open(fmt.Sprintf("https://tpix.typstify.com/packages/preview/%s", c.pkgInfo.Name))
}

func (t *PkgThumb) Layout(gtx C, th *theme.Theme) D {
	t.update(gtx)
	macro := op.Record(gtx.Ops)
	dims := layout.Stack{Alignment: layout.Center}.Layout(gtx,
		layout.Stacked(func(gtx C) D {
			return image.ImageStyle{
				Src:   t.thumb,
				Scale: 0.5, // ensure non-HDP screen is also scaled to 0.5.
			}.Layout(gtx)
		}),
		layout.Expanded(func(gtx C) D { return t.layoutForeground(gtx, th) }),
	)

	callOp := macro.Stop()

	defer clip.Rect(stdimg.Rectangle{Max: dims.Size}).Push(gtx.Ops).Pop()
	// register tag
	event.Op(gtx.Ops, t)
	t.click.Add(gtx.Ops)
	callOp.Add(gtx.Ops)

	return dims
}

func (t *PkgThumb) layoutForeground(gtx C, th *theme.Theme) D {
	if !t.hovering {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	rect := clip.Rect(stdimg.Rectangle{Max: gtx.Constraints.Max})
	paint.FillShape(gtx.Ops, misc.WithAlpha(th.Fg, th.HoverAlpha), rect.Op())
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

func (t *PkgThumb) update(gtx C) {
	for {
		event, ok := gtx.Event(
			pointer.Filter{Target: t, Kinds: pointer.Enter | pointer.Leave | pointer.Cancel},
		)
		if !ok {
			break
		}

		switch event := event.(type) {
		case pointer.Event:
			switch event.Kind {
			case pointer.Enter:
				t.hovering = true
			case pointer.Leave:
				t.hovering = false
			case pointer.Cancel:
				t.hovering = false
			}
		}
	}

	for {
		e, ok := t.click.Update(gtx.Source)
		if !ok {
			break
		}
		if e.Kind == gesture.KindClick && t.onClick != nil {
			t.onClick(t.rawImgLoc)
		}
	}
}

func layoutLabel(gtx C, th *theme.Theme, label string, value string) D {
	return layout.Inset{
		Top: unit.Dp(4),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Middle,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				lb := material.Label(th.Theme, th.TextSize*0.8, label+":")
				lb.Font.Weight = font.SemiBold
				return lb.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx C) D {
				if value == "" {
					value = "N/A"
				}
				lb := material.Label(th.Theme, th.TextSize*0.8, value)
				lb.Color = misc.WithAlpha(th.Fg, 0xf6)
				return lb.Layout(gtx)
			}),
		)
	})
}
