package dialog

import (
	"errors"
	"log"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/oligo/gioview/theme"
	"github.com/oligo/gioview/view"
	gw "github.com/oligo/gioview/widget"
	"looz.ws/typstify/i18n"
	"looz.ws/typstify/service"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/typst"
	"looz.ws/typstify/ui/statusbar"
	"looz.ws/typstify/widgets/icons"
)

type ProjectKind string

const (
	DocumentKind ProjectKind = "Document"
	PackageKind  ProjectKind = "Package"
	TemplateKind ProjectKind = "Template"
)

type ProjectCreateReq struct {
	Kind       ProjectKind
	ProjectDir string
	Name       string
	// template to use, for package or template kind this should be omitted.
	TemplateName string
}

type CreateProjectDialog struct {
	srv             *service.ServiceFacade
	kindEnum        widget.Enum
	templateInput   gw.TextField
	nameInput       gw.TextField
	projectDir      string
	projectDirInput gw.TextField
	openFolderBtn   widget.Clickable

	kindChoices []layout.FlexChild
}

var CreateProjectDialogViewID = view.NewViewID("CreateProjectDialogView")

var (
	tip1 = `Choose document if you want to write articles ,books, slides, etc. For package or template development, please choose the other two.`
	tip2 = `Find the desired template in Typst Packages explorer. Both local and remote templates can be used. 
A template name should have the form @namespace/package-name:version, for example: '@preview/aero-check:0.1.1'.
If you want to create project without template, just leave it empty.`
)

var projectKinds = []ProjectKind{DocumentKind, PackageKind, TemplateKind}

var (
	folderOpenIcon = icons.NewSvgIcon(icons.FolderOpen)
)

func NewCreateProjectDialog(srv *service.ServiceFacade) view.View {
	createDialog := &CreateProjectDialog{srv: srv}

	dialog := NewDialogModal(CreateProjectDialogViewID, i18n.Translate("Create New Project"), i18n.Translate("Create"))
	dialog.Dialog = createDialog
	return dialog
}

func (d *CreateProjectDialog) OnInit(intent view.Intent) error {
	d.kindEnum.Value = string(DocumentKind)
	val, ok := intent.Params["template"]
	if !ok {
		return nil
	}

	d.templateInput.SetText(val.(string))
	return nil
}

func (d *CreateProjectDialog) createDocumentProject(req *ProjectCreateReq) (string, error) {
	if req.Kind != DocumentKind {
		return "", nil
	}

	if req.Name == "" {
		return "", errors.New(i18n.Translate("Please set a name for your project."))
	}

	if req.ProjectDir == "" {
		return "", errors.New(i18n.Translate("Please open a folder in File Explorer to place the project files."))
	}

	if req.TemplateName != "" && !strings.HasPrefix(req.TemplateName, "@") {
		return "", errors.New(i18n.Translate("Please input a valid full template name."))
	}

	if req.TemplateName != "" {
		dir := filepath.Join(req.ProjectDir, req.Name)
		_, _, err := d.srv.PkgService().DownloadWithSpec(req.TemplateName)
		if err != nil {
			return "", err
		}
		err = typst.InitCmd(req.TemplateName, dir, &typst.InitCmdOptions{
			PackagePath:      d.srv.Settings().Typst().PackageDir,
			PackageCachePath: d.srv.Settings().Typst().PackageCacheDir,
		})
		if err != nil {
			return "", err
		}
		return dir, nil

	} else {
		return d.srv.PkgService().CreateSampleDocument(req.ProjectDir, req.Name)
	}

}

func (d *CreateProjectDialog) createPackageProject(req *ProjectCreateReq) (string, error) {
	if req.Kind != PackageKind && req.Kind != TemplateKind {
		return "", errors.New("not a package creation request")
	}

	if req.Name == "" {
		return "", errors.New(i18n.Translate("Please set a name for your package/template."))
	}

	dir, err := d.srv.PkgService().CreatePkg(req.ProjectDir, req.Name, req.Kind == TemplateKind)
	if err != nil {
		log.Println("create package error: ", err)
	}

	return dir, nil
}

func (d *CreateProjectDialog) OnConfirm() error {
	go func() {
		var req ProjectCreateReq
		var err error
		var dir string

		switch d.kindEnum.Value {
		case string(DocumentKind):
			req = ProjectCreateReq{
				Kind:         DocumentKind,
				Name:         d.nameInput.Text(),
				ProjectDir:   d.projectDir,
				TemplateName: d.templateInput.Text(),
			}
			dir, err = d.createDocumentProject(&req)
		case string(PackageKind):
			req = ProjectCreateReq{
				ProjectDir: d.projectDir,
				Kind:       PackageKind,
				Name:       d.nameInput.Text(),
			}

			dir, err = d.createPackageProject(&req)
		case string(TemplateKind):
			req = ProjectCreateReq{
				ProjectDir: d.projectDir,
				Kind:       TemplateKind,
				Name:       d.nameInput.Text(),
			}

			dir, err = d.createPackageProject(&req)
		}

		if err != nil {
			d.srv.EventBus().Emit(bus.TopicStatusbarNotifyEvent, statusbar.Notification{
				Content:  i18n.Translate("Create project error: %s", err.Error()),
				Level:    2,
				Duration: time.Second * 8,
			})
			return
		}

		d.srv.EventBus().Emit(bus.TopicProjectCreate, dir)
	}()

	return nil
}

func (d *CreateProjectDialog) LayoutBody(gtx C, th *theme.Theme) D {
	if d.openFolderBtn.Clicked(gtx) {
		go func() {
			d.projectDir, _ = d.srv.FileChooser().ChooseFolder()
		}()
	}

	if d.kindChoices == nil {
		for _, name := range projectKinds {
			name := name
			d.kindChoices = append(d.kindChoices, layout.Rigid(func(gtx C) D {
				return material.RadioButton(th.Theme, &d.kindEnum, string(name), string(name)).Layout(gtx)
			}))
		}
	}

	return layout.Flex{
		Axis: layout.Vertical,
	}.Layout(gtx,
		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Project Type"), i18n.Translate(strings.ReplaceAll(tip1, "\t", " ")),
				func(gtx C) D {
					return layout.Flex{
						Axis: layout.Horizontal,
					}.Layout(gtx, d.kindChoices...)
				})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Project Location"),
				i18n.Translate("Select the location where the project will be created."),
				func(gtx C) D {
					return d.openFolderBtn.Layout(gtx, func(gtx C) D {
						d.projectDirInput.Alignment = text.Start
						d.projectDirInput.SingleLine = true
						d.projectDirInput.State().ReadOnly = true
						d.projectDirInput.Leading = func(gtx layout.Context) layout.Dimensions {
							return folderOpenIcon.Layout(gtx, th.ContrastBg, th.TextSize*1.2)
						}
						if d.projectDir != d.projectDirInput.Text() {
							d.projectDirInput.SetText(d.projectDir)
						}

						return d.projectDirInput.Layout(gtx, th, "Select a directory")
					})
				})
		}),

		layout.Rigid(func(gtx C) D {
			if d.kindEnum.Value != string(DocumentKind) {
				return D{}
			}
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Typst Template"), i18n.Translate(tip2), func(gtx C) D {
				d.templateInput.Alignment = text.Start
				return d.templateInput.Layout(gtx, th, "")
			})
		}),

		layout.Rigid(func(gtx C) D {
			return formItem{Axis: layout.Vertical}.Layout(gtx, th, i18n.Translate("Project Name"),
				i18n.Translate("The name of your project. A new folder will be created inside of the directory selected."),
				func(gtx C) D {
					d.nameInput.Alignment = text.Start
					d.nameInput.SingleLine = true
					return d.nameInput.Layout(gtx, th, "")
				})
		}),
	)

}
