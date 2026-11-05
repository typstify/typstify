package pkgmgmt

import (
	"errors"
	"sync/atomic"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/misc"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gvwidget "github.com/oligo/gioview/widget"
	"github.com/typstify/tpix-cli/api"

	// gvwiget "github.com/oligo/gioview/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/typst/pkg"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/utils"
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
	srv              *service.ServiceFacade
	vm               view.ViewManager
	kindSelect       *widgets.Dropdown
	searchInput      gvwidget.TextField
	categoryList     *CategoryList
	packageList      *PkgList
	cards            []*PkgCard
	lastFetched      atomic.Pointer[pkgFetchResult]
	lastFetchedCount int
	lastFetchErr     error

	// searchSeq identifies the latest search, so the results of a slow, stale
	// search can be discarded.
	searchSeq       atomic.Uint64
	searchDebouncer *utils.Debouncer
}

// pkgFetchResult is the result of a package search. It is published atomically
// because the search runs on a background goroutine.
type pkgFetchResult struct {
	cards []*PkgCard
	count int
	err   error
}

func (vw *PkgListView) ID() view.ViewID {
	return PkgListViewID
}

func (vw *PkgListView) Title() string {
	return "Typst Packages"
}

func (vw *PkgListView) OnNavTo(intent view.Intent) error {
	vw.BaseView.OnNavTo(intent)

	vw.search(vw.kindSelect.Value(), vw.categoryList.GetChecked(), vw.searchInput.Text())

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

						var reqErr *api.RequestError
						if errors.As(vw.lastFetchErr, &reqErr) {
							return layout.Center.Layout(gtx, func(gtx C) D {
								return material.Label(th.Theme, th.TextSize, reqErr.Error()).Layout(gtx)
							})
						}
						return vw.packageList.Layout(gtx, th)
					}),
				)

			}),
		)
	})

}

func (vw *PkgListView) update(gtx C) {
	filtersChanged := vw.kindSelect.Update(gtx)
	if vw.categoryList.Update(gtx) {
		filtersChanged = true
	}
	typed := vw.searchInput.Changed() || vw.searchInput.Submitted()

	if filtersChanged || typed {
		kind := vw.kindSelect.Value()
		category := vw.categoryList.GetChecked()
		query := vw.searchInput.Text()

		// Show loading immediately while data loads in background
		vw.packageList = newPkgList(nil, true)

		if typed {
			// Debounce typing so we do not send a request for every keystroke.
			vw.searchDebouncer.Run(func() {
				vw.search(kind, category, query)
			})
		} else {
			vw.search(kind, category, query)
		}
	}

	if result := vw.lastFetched.Swap(nil); result != nil {
		vw.cards = result.cards
		vw.lastFetchedCount = result.count
		vw.lastFetchErr = result.err
		vw.packageList = newPkgList(vw.cards, false)
	}

}

// search fetches packages in the background. Only the result of the latest
// search is published, so the response of a slower, older query can not
// overwrite the results of a newer one.
func (vw *PkgListView) search(kind string, category string, query string) {
	seq := vw.searchSeq.Add(1)

	vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Query packages...")})

	go func() {
		result := &pkgFetchResult{}

		results, count, err := vw.srv.PkgService().SearchPkgs("", kind, category, query)
		if err != nil {
			result.err = err
			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Query packages failed: ") + err.Error()})
		} else {
			result.count = count
			result.cards = make([]*PkgCard, 0, len(results))
			for _, p := range results {
				result.cards = append(result.cards, newPkgCard(p, func(pkgInfo *pkg.TypstPkg) {
					vw.downloadPkg(pkgInfo)
				}))
			}

			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Typst packages info loaded."), Level: 0})
		}

		// A newer search has been started in the meantime, drop this result.
		if vw.searchSeq.Load() != seq {
			return
		}

		vw.lastFetched.Store(result)
		vw.srv.RefreshWindow()
	}()
}

func (vw *PkgListView) downloadPkg(pkgInfo *pkg.TypstPkg) {
	go func() {
		_, count, err := vw.srv.PkgService().Download(pkgInfo.Namespace, pkgInfo.Name, pkgInfo.LatestVersion)
		if err != nil {
			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: i18n.Translate("Download package failed: ") + err.Error(), Level: 2})
		} else {
			pkgSpec := pkgInfo.ImportPath()
			var msg string
			if count <= 1 {
				msg = i18n.Translate("Downloaded package %s. ", pkgSpec)
			} else {
				msg = i18n.Translate("Downloaded package %s and %d transitive dependencies.", pkgSpec, count-1)
			}
			vw.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{Content: msg, Level: 0})
		}
	}()
}

func (vw *PkgListView) LayoutStatus(gtx C, th *theme.Theme) D {
	pkgStatus := i18n.Translate("Found %d packages.", vw.lastFetchedCount)
	return material.Label(th.Theme, th.TextSize*0.9, pkgStatus).Layout(gtx)
}

func NewPkgListView(srv *service.ServiceFacade, vm view.ViewManager) view.View {
	kindSelect := widgets.NewDropDown(map[string]any{"all": "All", "pkg": "Package", "template": "Template"})
	// TPIX searches all kinds by default, the same as the tpix CLI.
	kindSelect.SetSelected("all")

	return &PkgListView{
		BaseView:     &view.BaseView{},
		srv:          srv,
		vm:           vm,
		categoryList: newCategoryList(),
		kindSelect:   kindSelect,
		packageList:  newPkgList(nil, true),
		searchDebouncer: &utils.Debouncer{
			Debounce: time.Millisecond * 300,
		},
	}
}
