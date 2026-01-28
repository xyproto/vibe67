//go:build darwin
// +build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type FileWatcher struct {
	kq          int
	watchMap    map[int]string
	mu          sync.Mutex
	debounceMap map[string]*time.Timer
	onChange    func(string)
}

func NewFileWatcher(onChange func(string)) (*FileWatcher, error) {
	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("kqueue failed: %v", err)
	}

	return &FileWatcher{
		kq:          kq,
		watchMap:    make(map[int]string),
		debounceMap: make(map[string]*time.Timer),
		onChange:    onChange,
	}, nil
}

func (fw *FileWatcher) AddFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	fd, err := unix.Open(absPath, unix.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %v", absPath, err)
	}

	event := unix.Kevent_t{
		Ident:  uint64(fd),
		Filter: unix.EVFILT_VNODE,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
		Fflags: unix.NOTE_WRITE | unix.NOTE_ATTRIB,
	}

	_, err = unix.Kevent(fw.kq, []unix.Kevent_t{event}, nil, nil)
	if err != nil {
		unix.Close(fd)
		return fmt.Errorf("failed to add kevent for %s: %v", absPath, err)
	}

	fw.mu.Lock()
	fw.watchMap[fd] = absPath
	fw.mu.Unlock()

	return nil
}

func (fw *FileWatcher) Watch() {
	events := make([]unix.Kevent_t, 10)

	for {
		n, err := unix.Kevent(fw.kq, nil, events, nil)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Error reading kevent: %v\n", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		for i := 0; i < n; i++ {
			event := events[i]
			fd := int(event.Ident)

			fw.mu.Lock()
			path := fw.watchMap[fd]
			fw.mu.Unlock()

			if path != "" {
				fw.debouncedCallback(path)
			}
		}
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
	fw.mu.Lock()
	defer fw.mu.Unlock()

	for fd := range fw.watchMap {
		unix.Close(fd)
	}

	return unix.Close(fw.kq)
}
