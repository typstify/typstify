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

type RecentProject struct {
	Path         string
	RelPath      string
	LastAccessAt time.Time
	TreeState    *filetree.TreeState
	OpenedFiles  []string
}

type RecentProjectsService struct {
	rootDir  string
	db       *bolt.DB
	index    *utils.Bucket[utils.SKey, RecentProject]
	Current  RecentProject
	allCache []RecentProject
}

func NewRecentProjectsService(rootDir string) *RecentProjectsService {
	db := openDB(filepath.Join(rootDir, "recent.db"))
	index := utils.NewBucket[utils.SKey]("recent-projects", db, &utils.JsonEncoder[RecentProject]{})

	return &RecentProjectsService{
		rootDir: rootDir,
		db:      db,
		index:   index,
	}
}

func (rp *RecentProjectsService) AddRecent(projectDir string) {
	project, err := rp.index.Get(utils.SKey(projectDir))
	if err == nil {
		rp.Current = project
		rp.Current.LastAccessAt = time.Now()
	} else {
		rp.Current = RecentProject{Path: projectDir, LastAccessAt: time.Now()}
	}
	rp.index.Save(utils.SKey(projectDir), rp.Current)
	rp.allCache = rp.allCache[:0] // invalidate cache
}

func (rp *RecentProjectsService) SaveSnapshot(treeState *filetree.TreeState, openedFiles []string) {
	if rp.Current.Path == "" {
		return
	}
	rp.Current.TreeState = treeState
	rp.Current.OpenedFiles = openedFiles
	rp.index.Save(utils.SKey(rp.Current.Path), rp.Current)
}

func (rp *RecentProjectsService) GetAll() []RecentProject {
	if len(rp.allCache) > 0 {
		return rp.allCache
	}

	all, err := rp.index.GetAll()
	if err != nil {
		log.Println("get recent projects failed: ", err)
		return nil
	}

	slices.SortFunc(all, func(a, b RecentProject) int {
		return -a.LastAccessAt.Compare(b.LastAccessAt)
	})

	rp.allCache = append(rp.allCache, all[:min(len(all), 100)]...)
	rp.shortenPaths(&rp.allCache)

	return rp.allCache
}

func (rp *RecentProjectsService) shortenPaths(all *[]RecentProject) {
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

func (rp *RecentProjectsService) Close() {
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
