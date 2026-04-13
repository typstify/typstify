package pkgmgmt

import (
	"sync/atomic"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gvwidget "github.com/oligo/gioview/widget"

	// gvwiget "github.com/oligo/gioview/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/typst/pkg"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets"
)

type (
	C = layout.Context
	D = layout.Dimensions
)

var (
	PkgListViewID = view.NewViewID("PkgListView")
	searchIcon, _ = widget.NewIcon(icons.ActionSearch)
)

type PkgListView struct {
	*view.BaseView
	srv          *service.ServiceFacade
	vm           view.ViewManager
	kindSelect   *widgets.Dropdown
	searchInput  gvwidget.TextField
	categoryList *CategoryList
	packageList  *PkgList
	cards        []*PkgCard
	lastFetched  atomic.Pointer[[]*PkgCard]
}

func (vw *PkgListView) ID() view.ViewID {
	return PkgListViewID
}

func (vw *PkgListView) Title() string {
	return "Typst Packages"
}

func (vw *PkgListView) OnNavTo(intent view.Intent) error {
	vw.BaseView.OnNavTo(intent)
	return nil
}

func (vw *PkgListView) Layout(gtx C, th *theme.Theme) D {
	vw.update(gtx)

	return layout.Inset{
		Top:    unit.Dp(36),
		Bottom: unit.Dp(36),
		Left:   unit.Dp(80),
		Right:  unit.Dp(80),
	}.Layout(gtx, func(gtx C) D {
		return layout.Flex{
			Axis:      layout.Horizontal,
			Alignment: layout.Start,
		}.Layout(gtx,
			layout.Rigid(func(gtx C) D {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(220))
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(180))

				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Inset{
							Right: unit.Dp(8),
						}.Layout(gtx, func(gtx C) D {
							return layout.Flex{
								Axis: layout.Vertical,
							}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									lb := material.Subtitle2(th.Theme, i18n.Translate("Package kind"))
									lb.Font.Weight = font.SemiBold
									return lb.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

								layout.Rigid(func(gtx C) D {
									return vw.kindSelect.Layout(gtx, th)
								}),
							)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx C) D {
						return vw.categoryList.Layout(gtx, th)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(24)}.Layout),
			layout.Flexed(1, func(gtx C) D {
				return layout.Flex{
					Axis: layout.Vertical,
				}.Layout(gtx,
					layout.Rigid(func(gtx C) D {
						return layout.Center.Layout(gtx, func(gtx C) D {
							return layout.Flex{
								Alignment: layout.Middle,
								Gap:       gtx.Dp(unit.Dp(8)),
							}.Layout(gtx,
								layout.Rigid(func(gtx C) D {
									return packageSearchIcon.Layout(gtx, th.Fg, th.TextSize*20.0/16.0)
								}),
								layout.Rigid(func(gtx C) D {
									return material.H6(th.Theme, i18n.Translate("Search packages/templates on TPIX")).Layout(gtx)
								}),
							)
						})

					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

					layout.Rigid(func(gtx C) D {
						return layout.Center.Layout(gtx, func(gtx C) D {
							label := material.Label(th.Theme, th.TextSize, i18n.Translate("Browsing thousands of packages and templates on TPIX server, including public namespaces, and private namespaces of your teams."))
							label.LineHeightScale = 1.5
							return label.Layout(gtx)
						})
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(48)}.Layout),
					layout.Rigid(func(gtx C) D {
						return layout.Inset{}.Layout(gtx, func(gtx C) D {
							vw.searchInput.SingleLine = true
							vw.searchInput.LabelOption = gvwidget.LabelOption{Alignment: gvwidget.Hidden}
							// vw.searchInput.MaxChars = 64
							vw.searchInput.Padding = unit.Dp(8)
							vw.searchInput.Leading = func(gtx C) D {
								return misc.Icon{Icon: searchIcon, Size: unit.Dp(18), Color: misc.WithAlpha(th.Fg, 0xb0)}.Layout(gtx, th)
							}
							return vw.searchInput.Layout(gtx, th, i18n.Translate("Search TPIX..."))
						})
					}),

					layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

					layout.Rigid(func(gtx C) D {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X

						return vw.packageList.Layout(gtx, th)
					}),
				)

			}),
		)
	})

}

func (vw *PkgListView) update(gtx C) {
	reload := false
	if vw.searchInput.Changed() || vw.searchInput.Submitted() {
		reload = true
	}
	if vw.categoryList.Update(gtx) {
		reload = true
	}
	if vw.kindSelect.Update(gtx) {
		reload = true
	}

	if reload {
		// Show loading immediately while data loads in background
		vw.packageList = newPkgList(nil, true)

		go func() {
			vw.loadData(vw.kindSelect.Value(), vw.categoryList.GetChecked(), vw.searchInput.Text())
		}()
	}

	if old := vw.lastFetched.Swap(nil); old != nil {
		vw.cards = *old
		vw.packageList = newPkgList(vw.cards, false)
	}

}

func (vw *PkgListView) loadData(kind string, category string, query string) {
	go func() {
		vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Query packages...")})

		cards := make([]*PkgCard, 0)

		results, err := vw.srv.PkgService().SearchPkgs("", kind, category, query)
		if err != nil {
			vw.lastFetched.Store(&cards)
			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Query packages failed: ") + err.Error()})
			return
		}

		for _, p := range results {
			card := newPkgCard(p,
				func(pkgInfo *pkg.TypstPkg) {
					vw.downloadPkg(pkgInfo)
				})

			cards = append(cards, card)
		}

		vw.lastFetched.Store(&cards)
		vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Typst packages info loaded."), Level: 0})
	}()
}

func (vw *PkgListView) downloadPkg(pkgInfo *pkg.TypstPkg) {
	go func() {
		count, err := vw.srv.PkgService().Download(pkgInfo.Namespace, pkgInfo.Name, pkgInfo.LatestVersion)
		if err != nil {
			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Download package failed: ") + err.Error(), Level: 2})
		} else {
			pkgSpec := pkgInfo.ImportPath()
			var msg string
			if count > 1 {
				msg = i18n.Translate("Downloaded package %s. ", pkgSpec)
			} else {
				msg = i18n.Translate("Downloaded package %s and %d transitive dependencies.", pkgSpec, count)
			}
			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: msg, Level: 0})
		}
	}()
}

func (vw *PkgListView) LayoutStatus(gtx C, th *theme.Theme) D {
	pkgStatus := i18n.Translate("Found %d packages.", len(vw.packageList.cards))
	return material.Label(th.Theme, th.TextSize*0.9, pkgStatus).Layout(gtx)
}

func NewPkgListView(srv *service.ServiceFacade, vm view.ViewManager) view.View {
	return &PkgListView{
		BaseView:     &view.BaseView{},
		srv:          srv,
		vm:           vm,
		categoryList: newCategoryList(),
		kindSelect:   widgets.NewDropDown(map[string]any{"all": "All", "pkg": "Package", "template": "Template"}),
		packageList:  newPkgList(nil, false),
	}
}
