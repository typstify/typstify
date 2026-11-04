package service

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	bolt "go.etcd.io/bbolt"
	"looz.ws/typstify/service/bus"
	"looz.ws/typstify/utils"
	"looz.ws/typstify/widgets/filetree"
)

type WorkspaceSettings struct {
	// preview mode: document|slide
	PreviewMode string `json:"preview_mode"`
}

type GitRepoState struct {
	// current git branch
	Branch      string
	Changes     []utils.GitFileChange
	AllBranches []utils.BranchInfo
}

type WorkspaceState struct {
	Path         string
	RelPath      string
	LastAccessAt time.Time
	TreeState    *filetree.TreeState
	OpenedFiles  []string
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

	appState AppState
	// mu guards the switching/reading of currentWorkspace as well as the
	// workspace settings file accessors.
	mu sync.Mutex

	fileWatcher *WorkspaceFileWatcher
	eventBus    *bus.EventBus

	gitRepoState *GitRepoState
	gitMu        sync.Mutex

	gitDebounceTimer *time.Timer
	gitRepoDirty     atomic.Bool
}

const gitDebounceInterval = 500 * time.Millisecond

func NewWorkspaceService(dataDir string, eventBus *bus.EventBus) *WorkspaceService {
	db := openDB(filepath.Join(dataDir, "recent.db"))
	stateIndex := utils.NewBucket[utils.SKey]("recent-projects", db, &utils.JsonEncoder[WorkspaceState]{})
	appStateIndex := utils.NewBucket[utils.SKey]("app-state", db, &utils.JsonEncoder[AppState]{})

	return &WorkspaceService{
		db:            db,
		stateIndex:    stateIndex,
		appStateIndex: appStateIndex,
		fileWatcher:   NewWorkspaceFileWatcher(eventBus),
		eventBus:      eventBus,
		gitRepoState:  &GitRepoState{},
	}
}

// SwitchWorkspace switches projects.
func (rp *WorkspaceService) SwitchWorkspace(projectDir string) {
	rp.mu.Lock()
	lastWorkspaceDir := rp.currentWorkspace.Path

	// restore or build states for the new workspace.
	project, err := rp.stateIndex.Get(utils.SKey(projectDir))
	if err == nil {
		rp.currentWorkspace = project
		rp.currentWorkspace.LastAccessAt = time.Now()
	} else {
		rp.currentWorkspace = WorkspaceState{Path: projectDir, LastAccessAt: time.Now()}
	}
	rp.stateIndex.Save(utils.SKey(projectDir), rp.currentWorkspace)
	rp.allCache = rp.allCache[:0] // invalidate cache
	rp.mu.Unlock()

	// unwatch last git repo if it is a git repo.
	if lastWorkspaceDir != "" {
		rp.unwatchGitRepo(lastWorkspaceDir)
	}

	rp.gitMu.Lock()
	rp.gitRepoState = &GitRepoState{}
	rp.gitMu.Unlock()
	if rp.updateGitRepoState(projectDir); rp.gitRepoState.Branch != "" {
		rp.watchGitRepo()
	}

}

func (rp *WorkspaceService) SaveSnapshot(treeState *filetree.TreeState, openedFiles []string) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

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

	if rp.fileWatcher != nil {
		rp.fileWatcher.Close()
	}

	if rp.gitDebounceTimer != nil {
		rp.gitDebounceTimer.Stop()
	}
}

// LoadWorkspaceSettings lookup settings.json in $projectDir/.typstify/settings.json
// and deserialize the json to WorkspaceSettings
// The settings file is tiny and read rarely, so it is read on every call
// instead of being cached. Caching it would require invalidating the cache on
// every workspace switch and every write.
func (rp *WorkspaceService) LoadWorkspaceSettings() WorkspaceSettings {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	return rp.loadWorkspaceSettingsForPath(rp.currentWorkspace.Path)
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

func (rp *WorkspaceService) SetPreviewMode(mode string) {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	projectDir := rp.currentWorkspace.Path
	if projectDir == "" {
		return
	}

	settings := rp.loadWorkspaceSettingsForPath(projectDir)
	settings.PreviewMode = mode
	rp.saveWorkspaceSettingForPath(projectDir, settings)
}

func (rp *WorkspaceService) WatchFile(path string) error {
	if rp.fileWatcher == nil {
		return nil
	}

	return rp.fileWatcher.WatchFile(path)
}

func (rp *WorkspaceService) UnwatchFile(path string) error {
	if rp.fileWatcher == nil {
		return nil
	}

	return rp.fileWatcher.UnwatchFile(path)
}

// watchGitRepo watches git branch switching, staging, etc., and emits events
// via eventbus. Editors or other components subscribe to these events to update
// their states.
func (rp *WorkspaceService) watchGitRepo() {
	if rp.currentWorkspace.Path == "" {
		return
	}

	workspaceRoot := rp.currentWorkspace.Path
	log.Println("watch git repo: ", workspaceRoot)

	if err := rp.WatchFile(filepath.Join(workspaceRoot, ".git", "HEAD")); err != nil {
		log.Println("watch git branch switch failed: ", err)
		return
	}

	if err := rp.WatchFile(filepath.Join(workspaceRoot, ".git", "index")); err != nil {
		log.Println("watch git staging failed: ", err)
		return
	}

	// watch for event bus events:
	rp.eventBus.Subscribe(rp, "workspace.gitrepo.changed", `git\..*`, func(topic string, data interface{}) {
		go rp.updateGitRepoState(workspaceRoot)
	})

	rp.eventBus.Subscribe(rp, "workspace.gitrepo.files", `workspace\.file\.changed`, func(topic string, data interface{}) {
		rp.scheduleGitUpdate()
	})

}

func (rp *WorkspaceService) unwatchGitRepo(workspaceDir string) {

	err := rp.UnwatchFile(filepath.Join(workspaceDir, ".git", "HEAD"))
	if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
		log.Println("unwatch file failed: ", err)
	}

	err = rp.UnwatchFile(filepath.Join(workspaceDir, ".git", "index"))
	if err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
		log.Println("unwatch file failed: ", err)
	}

	rp.eventBus.Unsubscribe(rp)
}

func (rp *WorkspaceService) updateGitRepoState(workspaceRoot string) {
	branch, err := utils.CurrentGitBranch(workspaceRoot)
	if err != nil {
		return
	}

	if branch == "" {
		// Detached HEAD (tag, remote branch, or bare commit) —
		// resolve to a human-readable name so the indicator stays
		// visible and the user can still switch to a real branch.
		branch = utils.HeadRefName(workspaceRoot)
	}

	allBranches, _ := utils.ListGitBranches(workspaceRoot)

	newState := &GitRepoState{
		Branch:      branch,
		Changes:     utils.GitRepoStatus(workspaceRoot),
		AllBranches: allBranches,
	}

	rp.gitMu.Lock()
	defer rp.gitMu.Unlock()
	rp.gitRepoState = newState
}

func (rp *WorkspaceService) GitRepo() GitRepoState {
	rp.gitMu.Lock()
	defer rp.gitMu.Unlock()
	return *rp.gitRepoState
}

// scheduleGitUpdate debounces workspace file changes so we only run
// expensive git commands (status, for-each-ref) once after changes settle.
func (rp *WorkspaceService) scheduleGitUpdate() {
	rp.gitRepoDirty.Store(true)

	rp.gitMu.Lock()
	defer rp.gitMu.Unlock()

	if rp.gitDebounceTimer != nil {
		rp.gitDebounceTimer.Stop()
	}
	rp.gitDebounceTimer = time.AfterFunc(gitDebounceInterval, func() {
		if rp.gitRepoDirty.CompareAndSwap(true, false) {
			go rp.updateGitRepoState(rp.currentWorkspace.Path)
		}
	})
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
