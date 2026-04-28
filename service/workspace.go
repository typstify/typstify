package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	cli "github.com/typstify/tpix-cli"
	bolt "go.etcd.io/bbolt"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/filetree"
)

type BibliographyExportMeta struct {
	Namespace  string `json:"namespace"`
	Library    string `json:"library"`
	Collection string `json:"collection"`
	Format     string `json:"format"`
}

type ManagedBibliography struct {
	File     string                 `json:"file"`      // file path relative to the project root.
	ExportID string                 `json:"export_id"` // exportID on TPIX server.
	Meta     BibliographyExportMeta `json:"meta"`
}

type WorkspaceSettings struct {
	// preview mode: document|slide
	PreviewMode string                `json:"preview_mode"`
	BibFiles    []ManagedBibliography `json:"managed_bibs"`
}

type WorkspaceState struct {
	Path         string
	RelPath      string
	LastAccessAt time.Time
	TreeState    *filetree.TreeState
	OpenedFiles  []string
	// current git branch
	GitBranch string
}

type AppState struct {
	WindowSize []int
}

type WorkspaceService struct {
	db               *bolt.DB
	stateIndex       *utils.Bucket[utils.SKey, WorkspaceState]
	appStateIndex    *utils.Bucket[utils.SKey, AppState]
	currentWorkspace WorkspaceState
	allCache         []WorkspaceState

	appState      AppState
	mu            sync.Mutex
	watcherCancel context.CancelFunc
	settingCache  *WorkspaceSettings
}

func NewWorkspaceService(dataDir string) *WorkspaceService {
	db := openDB(filepath.Join(dataDir, "recent.db"))
	stateIndex := utils.NewBucket[utils.SKey]("recent-projects", db, &utils.JsonEncoder[WorkspaceState]{})
	appStateIndex := utils.NewBucket[utils.SKey]("app-state", db, &utils.JsonEncoder[AppState]{})

	return &WorkspaceService{
		db:            db,
		stateIndex:    stateIndex,
		appStateIndex: appStateIndex,
	}
}

func (rp *WorkspaceService) AddRecent(projectDir string) {
	project, err := rp.stateIndex.Get(utils.SKey(projectDir))
	if err == nil {
		rp.currentWorkspace = project
		rp.currentWorkspace.LastAccessAt = time.Now()
	} else {
		rp.currentWorkspace = WorkspaceState{Path: projectDir, LastAccessAt: time.Now()}
	}
	rp.stateIndex.Save(utils.SKey(projectDir), rp.currentWorkspace)
	rp.allCache = rp.allCache[:0] // invalidate cache

	rp.restartWatcher()
	rp.clearSettingsCache()

	// detect if this project is a git repo, and its current branch.
	go func() {
		branch, err := utils.CurrentGitBranch(projectDir)
		if err != nil || branch == "" {
			return
		}

		rp.currentWorkspace.GitBranch = branch
	}()
}

func (rp *WorkspaceService) SaveSnapshot(treeState *filetree.TreeState, openedFiles []string) {
	if rp.currentWorkspace.Path == "" {
		return
	}
	rp.currentWorkspace.TreeState = treeState
	rp.currentWorkspace.OpenedFiles = openedFiles
	rp.stateIndex.Save(utils.SKey(rp.currentWorkspace.Path), rp.currentWorkspace)
}

func (rp *WorkspaceService) Current() *WorkspaceState {
	return &rp.currentWorkspace
}

func (rp *WorkspaceService) GetHistory(maxSize int) []WorkspaceState {
	if len(rp.allCache) > 0 {
		return rp.allCache
	}

	all, err := rp.stateIndex.GetAll()
	if err != nil {
		log.Println("get recent projects failed: ", err)
		return nil
	}

	slices.SortFunc(all, func(a, b WorkspaceState) int {
		return -a.LastAccessAt.Compare(b.LastAccessAt)
	})

	rp.allCache = append(rp.allCache, all[:min(len(all), maxSize)]...)
	rp.shortenPaths(&rp.allCache)

	return rp.allCache
}

func (rp *WorkspaceService) shortenPaths(all *[]WorkspaceState) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}
	homeDir = filepath.Clean(homeDir)

	for idx := range *all {
		projectPath := (*all)[idx].Path
		if strings.HasPrefix(projectPath, homeDir) {
			relPath, err := filepath.Rel(homeDir, projectPath)
			if err != nil {
				log.Printf("Error calculating relative path: %v\n", err)
			} else {
				(*all)[idx].RelPath = filepath.Join("~", relPath)
			}
		}
	}
}

var appStateKey = utils.SKey("app-state")

func (rp *WorkspaceService) RememberWindowSize(x, y int) {
	if x <= 0 || y <= 0 {
		return
	}

	rp.appState.WindowSize = []int{x, y}
}

func (rp *WorkspaceService) GetAppState() *AppState {
	if rp.appState.WindowSize == nil {
		appState, err := rp.appStateIndex.Get(appStateKey)
		if err != nil {
			return nil
		}
		rp.appState.WindowSize = appState.WindowSize
	}

	return &rp.appState
}

func (rp *WorkspaceService) Close() {
	rp.appStateIndex.Save(appStateKey, rp.appState)

	if rp.db != nil {
		rp.db.Close()
	}

	if rp.watcherCancel != nil {
		rp.watcherCancel()
	}
}

// LoadWorkspaceSettings lookup settings.json in $projectDir/.typstify/settings.json
// and deserialize the json to WorkspaceSettings
func (rp *WorkspaceService) LoadWorkspaceSettings() WorkspaceSettings {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if rp.currentWorkspace.Path == "" {
		return WorkspaceSettings{}
	}

	if rp.settingCache != nil {
		return *rp.settingCache
	}

	settings := rp.loadWorkspaceSettingsForPath(rp.currentWorkspace.Path)

	rp.settingCache = &settings

	return settings
}

// SaveWorkspaceSetting serialize setting to json and write to $projectDir/.typstify/settings.json
func (rp *WorkspaceService) saveWorkspaceSetting(setting WorkspaceSettings) {
	if rp.currentWorkspace.Path == "" {
		return
	}

	rp.saveWorkspaceSettingForPath(rp.currentWorkspace.Path, setting)
	rp.clearSettingsCache()
}

// SaveManagedBibliography saves the managed bibliography file info in settings.json.
func (rp *WorkspaceService) SaveManagedBibliography(bib ManagedBibliography) {
	if rp.currentWorkspace.Path == "" {
		return
	}

	existing := rp.LoadWorkspaceSettings()

	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Create the file if it does not exist yet, but never truncate existing content.
	err := rp.ensureManagedBibliographyFile(filepath.Join(rp.currentWorkspace.Path, bib.File))
	if err != nil {
		log.Printf("write bib file failed: %v", err)
	}

	existing.BibFiles = append(existing.BibFiles, bib)
	existing.BibFiles = slices.Compact(existing.BibFiles)

	rp.saveWorkspaceSetting(existing)
	// reset the watcher
	rp.restartWatcher()
}

func (rp *WorkspaceService) RemoveManagedBibliography(bibPath string) {
	if rp.currentWorkspace.Path == "" {
		return
	}

	existing := rp.LoadWorkspaceSettings()

	rp.mu.Lock()
	defer rp.mu.Unlock()

	existing.BibFiles = slices.DeleteFunc(
		existing.BibFiles,
		func(bib ManagedBibliography) bool {
			return bib.File == bibPath
		},
	)

	rp.saveWorkspaceSetting(existing)
	// reset the watcher
	rp.restartWatcher()
}

func (rp *WorkspaceService) restartWatcher() {
	rootDir := rp.currentWorkspace.Path
	if rootDir == "" {
		return
	}

	// stop the last watcher
	if rp.watcherCancel != nil {
		rp.watcherCancel()
	}

	// start bib sync watcher
	ctx, cancel := context.WithCancel(context.Background())
	rp.watcherCancel = cancel
	ticker := time.NewTicker(time.Minute * 3)

	syncBib := func() {
		settings := rp.loadWorkspaceSettingsForPath(rootDir)
		// remove obsolete files that have be deleted from the disk
		settings.BibFiles = slices.DeleteFunc(
			settings.BibFiles,
			func(bib ManagedBibliography) bool {
				if exists, _ := utils.CheckFileExists(filepath.Join(rootDir, bib.File)); !exists {
					if err := cli.DeleteZoteroExport(bib.ExportID, nil); err != nil {
						log.Println("delete zotero export failed: ", err)
					}

					return true
				}

				return false
			},
		)

		rp.mu.Lock()
		rp.saveWorkspaceSettingForPath(rootDir, settings)
		if rootDir == rp.currentWorkspace.Path {
			rp.clearSettingsCache()
		}
		rp.mu.Unlock()

		settings = rp.loadWorkspaceSettingsForPath(rootDir)

		for _, mb := range settings.BibFiles {
			if mb.ExportID != "" {
				err := rp.syncManagedBibliography(rootDir, mb)
				if err != nil {
					log.Println("export error: ", err)
					continue
				}

				log.Println("sync bibliography success from TPIX: ", mb.ExportID)
			}
		}
	}

	go func() {
		defer ticker.Stop()
		// sync once at start time.
		syncBib()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				syncBib()
			}
		}
	}()
	log.Println("restarted watcher")
}

func (rp *WorkspaceService) ensureManagedBibliographyFile(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	return file.Close()
}

func (rp *WorkspaceService) syncManagedBibliography(rootDir string, mb ManagedBibliography) error {
	filename := filepath.Join(rootDir, mb.File)
	if err := rp.ensureManagedBibliographyFile(filename); err != nil {
		return err
	}

	var content bytes.Buffer
	if err := cli.FetchZoteroExport(mb.ExportID, &content); err != nil {
		return err
	}

	if content.Len() == 0 {
		log.Printf("skip overwriting bibliography %s with empty content", mb.File)
		return nil
	}

	existingContent, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	if bytes.Equal(existingContent, content.Bytes()) {
		return nil
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	defer func() {
		if err := os.Remove(tmpName); err != nil && !os.IsNotExist(err) {
			log.Printf("remove temp bibliography file failed: %v", err)
		}
	}()

	if _, err := tmpFile.Write(content.Bytes()); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}

	return nil
}

func (rp *WorkspaceService) loadWorkspaceSettingsForPath(projectDir string) WorkspaceSettings {
	if projectDir == "" {
		return WorkspaceSettings{}
	}

	typstifyDir := filepath.Join(projectDir, ".typstify")
	settingsFile := filepath.Join(typstifyDir, "settings.json")

	err := os.MkdirAll(typstifyDir, 0755)
	if err != nil {
		log.Printf("create .typstify directory failed: %v", err)
		return WorkspaceSettings{}
	}

	data, err := os.ReadFile(settingsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return WorkspaceSettings{}
		}
		log.Printf("read workspace settings failed: %v", err)
		return WorkspaceSettings{}
	}

	var settings WorkspaceSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("parse workspace settings failed: %v", err)
		return WorkspaceSettings{}
	}

	return settings
}

func (rp *WorkspaceService) saveWorkspaceSettingForPath(projectDir string, setting WorkspaceSettings) {
	if projectDir == "" {
		return
	}

	typstifyDir := filepath.Join(projectDir, ".typstify")
	settingsFile := filepath.Join(typstifyDir, "settings.json")

	err := os.MkdirAll(typstifyDir, 0755)
	if err != nil {
		log.Printf("create .typstify directory failed: %v", err)
		return
	}

	data, err := json.MarshalIndent(setting, "", "  ")
	if err != nil {
		log.Printf("serialize workspace settings failed: %v", err)
		return
	}

	err = os.WriteFile(settingsFile, data, 0644)
	if err != nil {
		log.Printf("write workspace settings failed: %v", err)
	}
}

func (rp *WorkspaceService) clearSettingsCache() {
	rp.settingCache = nil
}

func (rp *WorkspaceService) SetPreviewMode(mode string) {
	if rp.currentWorkspace.Path == "" {
		return
	}

	existing := rp.LoadWorkspaceSettings()

	rp.mu.Lock()
	defer rp.mu.Unlock()

	existing.PreviewMode = mode
	rp.saveWorkspaceSetting(existing)
}

func openDB(dbFile string) *bolt.DB {
	baseDir := filepath.Dir(dbFile)
	err := os.MkdirAll(baseDir, 0700)
	if err != nil {
		log.Fatal(err)
	}

	// It will be created if it doesn't exist.
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	// don't forget to close it if needed
	return db
}
