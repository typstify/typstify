package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/typstify/tpix-cli"
	"github.com/typstify/tpix-cli/api"
	"looz.ws/typstify/utils"
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

type Bibliographies = []ManagedBibliography

type BibliographyService struct {
	workspacePath string
	tpixClient    *tpix.TpixSdk
	mu            sync.Mutex
	watcherCancel context.CancelFunc
	cache         Bibliographies
	// cached reports whether cache mirrors the on-disk state. It is needed
	// because an empty bibliography list has to be cached as well.
	cached bool
}

func NewBibliographyService(workspacePath string, tpixClient *tpix.TpixSdk) *BibliographyService {
	srv := &BibliographyService{
		workspacePath: workspacePath,
		tpixClient:    tpixClient,
	}
	srv.restartWatcher()
	return srv
}

// saveLocked serializes managed bibliographies to json and writes them to
// $projectDir/.typstify/bibliography.json. bs.mu must be held.
func (bs *BibliographyService) saveLocked(bib Bibliographies) {
	if bs.workspacePath == "" {
		return
	}

	typstifyDir := filepath.Join(bs.workspacePath, ".typstify")
	bibFile := filepath.Join(typstifyDir, "bibliography.json")

	err := os.MkdirAll(typstifyDir, 0755)
	if err != nil {
		log.Printf("create .typstify directory failed: %v", err)
		return
	}

	data, err := json.MarshalIndent(bib, "", "  ")
	if err != nil {
		log.Printf("serialize bibliography list failed: %v", err)
		return
	}

	err = os.WriteFile(bibFile, data, 0644)
	if err != nil {
		log.Printf("write bibliography list failed: %v", err)
		return
	}

	bs.cache = slices.Clone(bib)
	bs.cached = true
}

// LoadBibliographies returns the managed bibliographies of the workspace. The
// list is read from disk once and kept in sync with any write made by this
// service.
func (bs *BibliographyService) LoadBibliographies() Bibliographies {
	if bs.workspacePath == "" {
		return nil
	}

	bs.mu.Lock()
	defer bs.mu.Unlock()

	return bs.loadLocked()
}

// loadLocked returns a copy of the managed bibliographies, reading them from
// disk on the first call. bs.mu must be held.
func (bs *BibliographyService) loadLocked() Bibliographies {
	if !bs.cached {
		bs.cache = bs.readBibliographies(bs.workspacePath)
		bs.cached = true
	}

	return slices.Clone(bs.cache)
}

func (bs *BibliographyService) readBibliographies(projectDir string) Bibliographies {
	if projectDir == "" {
		return nil
	}

	typstifyDir := filepath.Join(projectDir, ".typstify")
	bibFile := filepath.Join(typstifyDir, "bibliography.json")

	err := os.MkdirAll(typstifyDir, 0755)
	if err != nil {
		log.Printf("create .typstify directory failed: %v", err)
		return nil
	}

	data, err := os.ReadFile(bibFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		log.Printf("read bibliography file failed: %v", err)
		return nil
	}

	var bibs Bibliographies
	if err := json.Unmarshal(data, &bibs); err != nil {
		log.Printf("parse bibliography file failed: %v", err)
		return nil
	}

	return bibs
}

// SaveManagedBibliography saves the managed bibliography file info in  bibliography.json.
func (bs *BibliographyService) SaveManagedBibliography(bib ManagedBibliography) {
	if bs.workspacePath == "" {
		return
	}

	// Create the file if it does not exist yet, but never truncate existing content.
	err := bs.ensureManagedBibliographyFile(filepath.Join(bs.workspacePath, bib.File))
	if err != nil {
		log.Printf("write bib file failed: %v", err)
	}

	bs.mu.Lock()
	existing := append(bs.loadLocked(), bib)
	slices.SortStableFunc(existing, func(a, b ManagedBibliography) int {
		return strings.Compare(a.ExportID, b.ExportID)
	})
	existing = slices.Compact(existing)
	bs.saveLocked(existing)
	bs.mu.Unlock()

	// reset the watcher
	bs.restartWatcher()
}

func (bs *BibliographyService) RemoveManagedBibliography(bibPath string) {
	if bs.workspacePath == "" {
		return
	}

	bs.mu.Lock()
	existing := slices.DeleteFunc(
		bs.loadLocked(),
		func(bib ManagedBibliography) bool {
			return bib.File == bibPath
		},
	)
	bs.saveLocked(existing)
	bs.mu.Unlock()

	// reset the watcher
	bs.restartWatcher()
}

func (bs *BibliographyService) restartWatcher() {
	rootDir := bs.workspacePath
	if rootDir == "" {
		return
	}

	// stop the last watcher
	bs.mu.Lock()
	if bs.watcherCancel != nil {
		bs.watcherCancel()
	}

	// start bib sync watcher
	ctx, cancel := context.WithCancel(context.Background())
	bs.watcherCancel = cancel
	bs.mu.Unlock()

	ticker := time.NewTicker(time.Minute * 3)

	syncBib := func() {
		var deleted Bibliographies

		bs.mu.Lock()
		existing := bs.loadLocked()
		origLen := len(existing)
		// remove obsolete files that have be deleted from the disk
		bibs := slices.DeleteFunc(
			existing,
			func(bib ManagedBibliography) bool {
				exists, _ := utils.CheckFileExists(filepath.Join(rootDir, bib.File))
				if !exists {
					deleted = append(deleted, bib)
				}

				return !exists
			},
		)
		// Only touch the file if something actually got removed, otherwise
		// opening a project would create an empty bibliography.json.
		if len(bibs) != origLen {
			bs.saveLocked(bibs)
		}
		bs.mu.Unlock()

		// Drop the remote exports outside the lock to avoid blocking readers
		// while a request is in flight.
		for _, bib := range deleted {
			if bib.ExportID == "" {
				continue
			}

			if err := bs.tpixClient.DeleteZoteroExport(bib.ExportID); err != nil {
				log.Println("delete zotero export failed: ", err)
			}
		}

		for _, mb := range bibs {
			if mb.ExportID != "" {
				err := bs.syncManagedBibliography(rootDir, mb)
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

func (bs *BibliographyService) ensureManagedBibliographyFile(filename string) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}

	return file.Close()
}

func (bs *BibliographyService) syncManagedBibliography(rootDir string, mb ManagedBibliography) error {
	filename := filepath.Join(rootDir, mb.File)
	if err := bs.ensureManagedBibliographyFile(filename); err != nil {
		return err
	}

	var content bytes.Buffer
	if err := bs.tpixClient.FetchZoteroExport(mb.ExportID, &content); err != nil {
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

func (bs *BibliographyService) ListZoteroLibraries() ([]api.ZoteroLibrary, error) {
	return bs.tpixClient.ListZoteroLibraries()
}

func (bs *BibliographyService) CreateZoteroExport(parentDir string, filename string, library api.ZoteroLibrary, selectedCollectionKey string) error {
	var libraryID int
	var namespaceID, namespaceName, libraryName, collectionName string
	var scope string

	for _, collection := range library.Collections {
		if collection.Key == selectedCollectionKey {
			namespaceID = library.NamespaceID
			libraryID = library.Library.ID
			scope = library.Scope
			namespaceName = library.Namespace
			libraryName = library.Library.Name
			collectionName = collection.Name
		}
	}

	if filename == "" {
		filename = fmt.Sprintf("%s-%s-%s.bib", namespaceName, libraryName, collectionName)
	}

	var format = "biblatex"
	if strings.HasSuffix(filename, ".yml") || strings.HasSuffix(filename, ".yaml") {
		format = "hayagriva"
	}

	exportID, err := bs.tpixClient.CreateZoteroExport(filename, namespaceID, scope, int64(libraryID), selectedCollectionKey, format)
	if err != nil {
		return err
	}

	bibFile := ManagedBibliography{
		ExportID: exportID,
		File:     filepath.Join(parentDir, filename),
		Meta: BibliographyExportMeta{
			Namespace:  namespaceName,
			Library:    libraryName,
			Collection: collectionName,
			Format:     format,
		},
	}

	bs.SaveManagedBibliography(bibFile)
	return nil
}

func (bs *BibliographyService) RemoveBibliography(meta ManagedBibliography) error {
	err := bs.tpixClient.DeleteZoteroExport(meta.ExportID)
	if err != nil {
		return err
	}

	bs.RemoveManagedBibliography(meta.File)
	return nil
}

func (bs *BibliographyService) Close() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.watcherCancel != nil {
		bs.watcherCancel()
	}
}
