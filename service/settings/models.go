package settings

import (
	"fmt"
	"os"

	"gioui.org/font"
)

type Model interface {
	Load() error
	Save() error
	Validate() error
	// Keys() []string
}

var (
	_ Model = (*GeneralSettings)(nil)
	_ Model = (*EditorSettings)(nil)
	_ Model = (*TypstSettings)(nil)
	_ Model = (*TpixSettings)(nil)
	_ Model = (*AcpAgentSettings)(nil)
)

var (
	defaultGeneralSettings  *GeneralSettings
	defaultEditorSettings   *EditorSettings
	defaultTypstSettings    *TypstSettings
	defaultAcpAgentSettings *AcpAgentSettings
)

type GeneralSettings struct {
	baseModel

	// root dir for user data
	RootDir     string  `key:"rootDir" json:"rootDir"`
	Language    string  `key:"language" json:"language"`
	TextSize    float32 `key:"textSize" json:"textSize"`
	TypeFace    string  `key:"fontType" json:"fontType"`
	Theme       string  `key:"theme" json:"theme"`
	CheckUpdate string  `key:"checkUpdate" json:"checkUpdate"`
	DeviceID    string  `key:"deviceId" json:"deviceId"`

	EnableLSPLogs        int    `key:"enableLspLogs" json:"enableLspLogs"`
	EnablePowerSaving    int    `key:"enablePowerSaving" json:"enablePowerSaving"`
	ExternalTypst        string `key:"externalTypst" json:"externalTypst"`       // typst executable path
	ExternalTinymist     string `key:"externalTinymist" json:"externalTinymist"` // tinymist executable path
	OpenPreviewInBrowser int    `key:"openPreviewInBrowser" json:"openPreviewInBrowser"`
}

type EditorSettings struct {
	baseModel
	// typeface for editing
	TypeFace         string  `key:"fontType" json:"fontType"`
	TextSize         float32 `key:"fontSize" json:"fontSize"`
	Weight           int     `key:"fontWeight" json:"fontWeight"`
	LineHeightScale  float32 `key:"lineHeightScale" json:"lineHeightScale"`
	TabSize          int     `key:"tabSize" json:"tabSize"`
	UseSoftTab       string  `key:"softTab" json:"softTab"`
	WrapLine         string  `key:"wrapLine" json:"wrapLine"`
	AutoSaveInterval int     `key:"autoSaveInterval" json:"autoSaveInterval"`
}

type TypstSettings struct {
	baseModel
	Version             string `key:"version" json:"version"`
	PackageCacheDir     string `key:"cacheDir" json:"cacheDir"`
	PackageDir          string `key:"localPkgDir" json:"localPkgDir"`
	ExtraFontPath       string `key:"extraFontPath" json:"extraFontPath"`
	UseSysInputs        int    `key:"useSysInputs" json:"useSysInputs"`
	IgnoreSystemFonts   int    `key:"ignoreSystemFonts" json:"ignoreSystemFonts"`
	IgnoreEmbeddedFonts int    `key:"ignoreEmbeddedFonts" json:"ignoreEmbeddedFonts"`
	BuildDeps           int    `key:"buildDeps" json:"buildDeps"`
	OutputDir           string `key:"outputDir" json:"outputDir"`
}

type TpixSettings struct {
	baseModel

	Username     string `key:"username" json:"username"`
	Email        string `key:"email" json:"email"`
	AccessToken  string `key:"accessToken" json:"accessToken"`
	RefreshToken string `key:"refreshToken" json:"refreshToken"`
	LoginAt      int64  `key:"loginAt" json:"loginAt"`
}

type AcpAgentSettings struct {
	baseModel

	AgentID   string `key:"agentId" json:"agentId"`     // registry ID, or empty for custom
	AgentName string `key:"agentName" json:"agentName"` // display name
	Cmd       string `key:"cmd" json:"cmd"`             // resolved command, e.g. "npx"
	Args      string `key:"args" json:"args"`           // resolved args, space-separated
	Env       string `key:"env" json:"env"`             // extra env vars, space-separated KEY=value pairs

	// MCP server
	UseStaticMcpPort int `key:"useStaticMcpPort" json:"useStaticMcpPort"` // use fixed port when starting MCP server.
}

func (s *AcpAgentSettings) Validate() error { return nil }
func (s *AcpAgentSettings) Save() error     { return s.save(s) }
func (s *AcpAgentSettings) Load() error     { return s.load(s, defaultAcpAgentSettings) }

func (g *GeneralSettings) Save() error {
	if err := g.Validate(); err != nil {
		return err
	}

	return g.baseModel.save(g)
}

func (g *GeneralSettings) Load() error {
	return g.baseModel.load(g, defaultGeneralSettings)
}

func (g *GeneralSettings) Validate() error {
	// Load persisted values to determine what actually changed
	persisted := &GeneralSettings{}
	g.baseModel.loadPersisted(persisted)

	// Validate RootDir if changed
	if g.RootDir != persisted.RootDir {
		if err := isDir(g.RootDir); err != nil {
			return fmt.Errorf("%s is not a directory", g.RootDir)
		}
	}

	// Validate Language if changed
	if g.Language != persisted.Language {
		if g.Language == "" {
			return fmt.Errorf("Language is not set")
		}
	}

	// DeviceID is read-only, always validate
	if g.DeviceID == "" {
		return fmt.Errorf("DeviceID is missing")
	}

	// Validate ExternalTypst if changed
	if g.ExternalTypst != persisted.ExternalTypst {
		if g.ExternalTypst != "" {
			if err := isFile(g.ExternalTypst); err != nil {
				return err
			}
		}
	}

	// Validate ExternalTinymist if changed
	if g.ExternalTinymist != persisted.ExternalTinymist {
		if g.ExternalTinymist != "" {
			if err := isFile(g.ExternalTinymist); err != nil {
				return err
			}
		}
	}

	return nil
}

func (e *EditorSettings) Save() error {
	if err := e.Validate(); err != nil {
		return err
	}

	return e.baseModel.save(e)
}

func (e *EditorSettings) Load() error {
	return e.baseModel.load(e, defaultEditorSettings)
}

func (e *EditorSettings) Validate() error {
	// Load persisted values to determine what actually changed
	persisted := &EditorSettings{}
	e.baseModel.loadPersisted(persisted)

	e2 := defaultEditorSettings

	// Apply default for LineHeightScale if changed and invalid
	if e.LineHeightScale != persisted.LineHeightScale && e.LineHeightScale <= 0 {
		e.LineHeightScale = e2.LineHeightScale
	}

	// Apply default for TextSize if changed and invalid
	if e.TextSize != persisted.TextSize && e.TextSize <= 0 {
		e.TextSize = e2.TextSize
	}

	// Apply default for Weight if changed and out of range
	if e.Weight != persisted.Weight {
		if font.Weight(e.Weight) < font.Thin || font.Weight(e.Weight) > font.Black {
			e.Weight = e2.Weight
		}
	}

	// Apply default for TypeFace if changed and empty
	if e.TypeFace != persisted.TypeFace && e.TypeFace == "" {
		e.TypeFace = e2.TypeFace
	}

	// Apply default for TabSize if changed and invalid
	if e.TabSize != persisted.TabSize && e.TabSize <= 0 {
		e.TabSize = e2.TabSize
	}

	// Apply default for UseSoftTab if changed and empty
	if e.UseSoftTab != persisted.UseSoftTab && e.UseSoftTab == "" {
		e.UseSoftTab = e2.UseSoftTab
	}
	if e.WrapLine != persisted.WrapLine && e.WrapLine == "" {
		e.WrapLine = e2.WrapLine
	}

	// Validate AutoSaveInterval if changed - must be > 0
	if e.AutoSaveInterval != persisted.AutoSaveInterval && e.AutoSaveInterval <= 0 {
		return fmt.Errorf("AutoSaveInterval should > 0")
	}

	return nil
}

func (t *TypstSettings) Save() error {
	if err := t.Validate(); err != nil {
		return err
	}
	return t.baseModel.save(t)
}

func (t *TypstSettings) Load() error {
	return t.baseModel.load(t, defaultTypstSettings)
}

func (t *TypstSettings) Validate() error {
	// Load persisted values to determine what actually changed
	persisted := &TypstSettings{}
	t.baseModel.loadPersisted(persisted)

	// Validate PackageCacheDir if changed
	if t.PackageCacheDir != persisted.PackageCacheDir && t.PackageCacheDir != "" {
		if err := isDir(t.PackageCacheDir); err != nil {
			return err
		}
	}

	// Validate PackageDir if changed
	if t.PackageDir != persisted.PackageDir && t.PackageDir != "" {
		if err := isDir(t.PackageDir); err != nil {
			return err
		}
	}

	// Validate ExtraFontPath if changed
	if t.ExtraFontPath != persisted.ExtraFontPath && t.ExtraFontPath != "" {
		if err := isDir(t.ExtraFontPath); err != nil {
			return err
		}
	}

	// Validate OutputDir if changed
	if t.OutputDir != persisted.OutputDir && t.OutputDir != "" {
		if err := isDir(t.OutputDir); err != nil {
			return err
		}
	}

	return nil
}

func (t *TpixSettings) Save() error {
	if err := t.Validate(); err != nil {
		return err
	}

	return t.baseModel.save(t)
}

func (t *TpixSettings) Load() error {
	return t.baseModel.load(t, &TpixSettings{})
}

func (t *TpixSettings) Validate() error {
	return nil
}

func (t *TpixSettings) Clear() {
	t.Username = ""
	t.Email = ""
	t.AccessToken = ""
	t.RefreshToken = ""
	t.LoginAt = 0
	t.Save()
}

func init() {
	// do a initialize here:
	defaultGeneralSettings = &GeneralSettings{
		RootDir:              configRoot(),
		Language:             "en-US",
		DeviceID:             genDeviceID(),
		Theme:                "Default Light",
		TextSize:             13,
		TypeFace:             "",
		CheckUpdate:          "true",
		EnableLSPLogs:        0,
		EnablePowerSaving:    0,
		OpenPreviewInBrowser: 0,
	}

	defaultEditorSettings = &EditorSettings{
		TypeFace:         "Hack, Roboto Mono, Go Mono, Noto Emoji, monospace",
		TextSize:         13,
		Weight:           int(font.Normal),
		LineHeightScale:  1.6,
		UseSoftTab:       "true",
		TabSize:          4,
		WrapLine:         "true",
		AutoSaveInterval: 3,
	}

	defaultTypstSettings = &TypstSettings{
		Version:             "",
		PackageCacheDir:     "",
		PackageDir:          "",
		IgnoreSystemFonts:   0,
		IgnoreEmbeddedFonts: 0,
		UseSysInputs:        1,
		ExtraFontPath:       "",
		BuildDeps:           0,
		OutputDir:           "",
	}

	defaultAcpAgentSettings = &AcpAgentSettings{
		AgentID:          "claude-acp",
		AgentName:        "Claude Code",
		Cmd:              "npx",
		Args:             "-y @agentclientprotocol/claude-agent-acp@0.35.0",
		UseStaticMcpPort: 0, // not-fixed, or 1 for static port.
	}
}

func isDir(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	return nil
}

func isFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s cannot be accessed", path)
	}

	if st.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}

	return nil
}
