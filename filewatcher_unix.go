// Completion: 100% - Platform-specific module complete
//go:build linux
// +build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

type FileWatcher struct {
	fd          int
	watchMap    map[int]string
	mu          sync.Mutex
	debounceMap map[string]*time.Timer
	onChange    func(string)
}

func NewFileWatcher(onChange func(string)) (*FileWatcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("inotify_init failed: %v", err)
	}

	return &FileWatcher{
		fd:          fd,
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

	wd, err := unix.InotifyAddWatch(fw.fd, absPath, unix.IN_MODIFY|unix.IN_CLOSE_WRITE)
	if err != nil {
		return fmt.Errorf("failed to watch %s: %v", absPath, err)
	}

	fw.mu.Lock()
	fw.watchMap[wd] = absPath
	fw.mu.Unlock()

	return nil
}

func (fw *FileWatcher) Watch() {
	buf := make([]byte, unix.SizeofInotifyEvent*10)

	for {
		n, err := unix.Read(fw.fd, buf)
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			if VerboseMode {
				fmt.Fprintf(os.Stderr, "Error reading inotify events: %v\n", err)
			}
			continue
		}

		offset := 0
		for offset < n {
			event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[offset]))
			offset += unix.SizeofInotifyEvent + int(event.Len)

			if event.Mask&(unix.IN_MODIFY|unix.IN_CLOSE_WRITE) != 0 {
				fw.mu.Lock()
				path := fw.watchMap[int(event.Wd)]
				fw.mu.Unlock()

				if path != "" {
					fw.debouncedCallback(path)
				}
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
	return unix.Close(fw.fd)
}
