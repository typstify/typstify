package service

import (
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/filetree"
)

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

	appState AppState
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
