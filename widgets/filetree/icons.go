package filetree

import (
	"path/filepath"

	"looz.ws/typstify/widgets/icons"
)

var (
	TextFileIcon        = icons.NewSvgIcon(icons.FileText)
	TypeSettingFileIcon = icons.NewSvgIcon(icons.FileType)
	CodeFileIcon        = icons.NewSvgIcon(icons.FileCode)
	ImageFileIcon       = icons.NewSvgIcon(icons.FileImage)
	BinaryFileIcon      = icons.NewSvgIcon(icons.FileBinary)
	FolderIcon          = icons.NewSvgIcon(icons.ChevronRight)
	FolderOpenIcon      = icons.NewSvgIcon(icons.ChevronDown)
)

func ChooseFileIcon(name string) *icons.SvgIcon {
	ext := filepath.Ext(name)
	switch ext {
	case ".typ":
		return TypeSettingFileIcon
	case ".png", ".jpg", ".jpeg", ".gif", ".PNG", ".JPG", ".JPEG", ".GIF", ".webp":
		return ImageFileIcon
	case ".js", ".ts", ".py", ".go", ".java", ".rs", ".html", ".css", ".c", ".cpp", ".bib", ".json", ".yaml", ".yml", ".toml", ".csv":
		return CodeFileIcon
	case ".wasm", ".exe", ".bin":
		return BinaryFileIcon
	default:
		return TextFileIcon
	}
}
