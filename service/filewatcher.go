package service

import (
	"log"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"looz.ws/typstify/service/bus"
)

type WorkspaceFileWatcher struct {
	eventbus *bus.EventBus
	watcher  *fsnotify.Watcher

	mu           sync.Mutex
	watchedFiles map[string]int
	watchedDirs  map[string]int
}

func NewWorkspaceFileWatcher(eventbus *bus.EventBus) *WorkspaceFileWatcher {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	wfw := &WorkspaceFileWatcher{
		eventbus:     eventbus,
		watcher:      watcher,
		watchedFiles: make(map[string]int),
		watchedDirs:  make(map[string]int),
	}

	go wfw.run()

	return wfw
}

func (w *WorkspaceFileWatcher) WatchFile(path string) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)

	w.mu.Lock()
	defer w.mu.Unlock()

	w.watchedFiles[cleanPath]++
	if w.watchedDirs[dir] == 0 {
		if err := w.watcher.Add(dir); err != nil {
			delete(w.watchedFiles, cleanPath)
			return err
		}
	}
	w.watchedDirs[dir]++

	return nil
}

func (w *WorkspaceFileWatcher) UnwatchFile(path string) error {
	cleanPath := filepath.Clean(path)
	dir := filepath.Dir(cleanPath)

	w.mu.Lock()
	defer w.mu.Unlock()

	if cnt := w.watchedFiles[cleanPath]; cnt <= 1 {
		delete(w.watchedFiles, cleanPath)
	} else {
		w.watchedFiles[cleanPath] = cnt - 1
	}

	if cnt := w.watchedDirs[dir]; cnt <= 1 {
		delete(w.watchedDirs, dir)
		return w.watcher.Remove(dir)
	} else if cnt > 1 {
		w.watchedDirs[dir] = cnt - 1
	}

	return nil
}

func (w *WorkspaceFileWatcher) Close() error {
	return w.watcher.Close()
}

func (w *WorkspaceFileWatcher) run() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Println("workspace file watcher error:", err)
		}
	}
}

func (w *WorkspaceFileWatcher) handleEvent(event fsnotify.Event) {
	if !(event.Op.Has(fsnotify.Create) || event.Op.Has(fsnotify.Write) || event.Op.Has(fsnotify.Remove) || event.Op.Has(fsnotify.Rename)) {
		return
	}

	cleanPath := filepath.Clean(event.Name)

	w.mu.Lock()
	_, watched := w.watchedFiles[cleanPath]
	w.mu.Unlock()

	if !watched {
		return
	}

	// Emit git-specific events for .git repo state changes, skip the
	// generic workspace.file.changed event for these internal files.
	if filepath.Base(filepath.Dir(cleanPath)) == ".git" {
		switch filepath.Base(cleanPath) {
		case "HEAD":
			w.eventbus.Emit(bus.TopicGitBranchChanged, nil)
		case "index":
			w.eventbus.Emit(bus.TopicGitFileStaged, nil)
		}
		return
	}

	w.eventbus.Emit(bus.TopicWorkspaceFileChanged, bus.FileChangedEvent{Path: cleanPath})
}
