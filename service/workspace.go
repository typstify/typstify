package service

import (
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
	BibFiles []ManagedBibliography `json:"managed_bibs"`
}

type WorkspaceState struct {
	Path         string
	RelPath      string
	LastAccessAt time.Time
	TreeState    *filetree.TreeState
	OpenedFiles  []string
}

type AppState struct {
	WindowSize   []int
	WorkspaceDir string
}

type WorkspaceService struct {
	db               *bolt.DB
	stateIndex       *utils.Bucket[utils.SKey, WorkspaceState]
	appStateIndex    *utils.Bucket[utils.SKey, AppState]
	currentWorkspace WorkspaceState
	allCache         []WorkspaceState

	appState      AppState
	watcherTicker *time.Ticker
	mu            sync.Mutex
	watcherCancel context.CancelFunc
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

	// stop the last watcher
	if rp.watcherCancel != nil {
		rp.watcherCancel()
	}
	// start bib sync watcher
	ctx, cancel := context.WithCancel(context.Background())
	rp.watcherCancel = cancel
	rp.startWatcher(ctx)
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

	if rp.watcherTicker != nil {
		rp.watcherTicker.Stop()
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

	typstifyDir := filepath.Join(rp.currentWorkspace.Path, ".typstify")
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

// SaveWorkspaceSetting serialize setting to json and write to $projectDir/.typstify/settings.json
func (rp *WorkspaceService) SaveWorkspaceSetting(setting WorkspaceSettings) {
	existing := rp.LoadWorkspaceSettings()

	rp.mu.Lock()
	defer rp.mu.Unlock()

	existing.BibFiles = append(existing.BibFiles, setting.BibFiles...)
	existing.BibFiles = slices.Compact(existing.BibFiles)

	if rp.currentWorkspace.Path == "" {
		return
	}

	typstifyDir := filepath.Join(rp.currentWorkspace.Path, ".typstify")
	settingsFile := filepath.Join(typstifyDir, "settings.json")

	err := os.MkdirAll(typstifyDir, 0755)
	if err != nil {
		log.Printf("create .typstify directory failed: %v", err)
		return
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		log.Printf("serialize workspace settings failed: %v", err)
		return
	}

	err = os.WriteFile(settingsFile, data, 0644)
	if err != nil {
		log.Printf("write workspace settings failed: %v", err)
	}
}

func (rp *WorkspaceService) startWatcher(ctx context.Context) {
	rp.watcherTicker = time.NewTicker(time.Minute * 5)
	rootDir := rp.currentWorkspace.Path
	if rootDir == "" {
		return
	}

	syncBib := func() {
		settings := rp.LoadWorkspaceSettings()
		for _, mb := range settings.BibFiles {
			if mb.ExportID != "" {
				file, err := os.OpenFile(filepath.Join(rootDir, mb.File), os.O_RDWR|os.O_CREATE, 0644)
				if err != nil {
					log.Println("open file error: ", err)
					return
				}
				err = cli.FetchZoteroExport(mb.ExportID, file)
				if err != nil {
					log.Println("export error: ", err)
					return
				}

				log.Println("sync bibliography success from TPIX: ", mb.ExportID)
			}
		}
	}
	// sync once at start time.
	go syncBib()

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-rp.watcherTicker.C:
			syncBib()
		}
	}()

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
