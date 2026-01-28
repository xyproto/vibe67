//go:build windows
// +build windows

package main

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FileWatcher struct {
	watchMap    map[string]time.Time
	mu          sync.Mutex
	debounceMap map[string]*time.Timer
	onChange    func(string)
	stopChan    chan struct{}
}

func NewFileWatcher(onChange func(string)) (*FileWatcher, error) {
	return &FileWatcher{
		watchMap:    make(map[string]time.Time),
		debounceMap: make(map[string]*time.Timer),
		onChange:    onChange,
		stopChan:    make(chan struct{}),
	}, nil
}

func (fw *FileWatcher) AddFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	fw.mu.Lock()
	fw.watchMap[absPath] = time.Time{}
	fw.mu.Unlock()

	return nil
}

func (fw *FileWatcher) Watch() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fw.checkFiles()
		case <-fw.stopChan:
			return
		}
	}
}

func (fw *FileWatcher) checkFiles() {
	fw.mu.Lock()
	paths := make([]string, 0, len(fw.watchMap))
	for path := range fw.watchMap {
		paths = append(paths, path)
	}
	fw.mu.Unlock()

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		fw.mu.Lock()
		lastMod := fw.watchMap[path]
		fw.mu.Unlock()

		if !lastMod.IsZero() && info.ModTime().After(lastMod) {
			fw.debouncedCallback(path)
		}

		fw.mu.Lock()
		fw.watchMap[path] = info.ModTime()
		fw.mu.Unlock()
	}
}

func (fw *FileWatcher) debouncedCallback(path string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if timer, exists := fw.debounceMap[path]; exists {
		timer.Stop()
	}

	fw.debounceMap[path] = time.AfterFunc(500*time.Millisecond, func() {
		fw.onChange(path)
		fw.mu.Lock()
		delete(fw.debounceMap, path)
		fw.mu.Unlock()
	})
}

func (fw *FileWatcher) Close() error {
	close(fw.stopChan)
	return nil
}
