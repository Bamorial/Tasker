package tui

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/bamorial/tasker/internal/tasker"
)

const fileWatchDebounce = 125 * time.Millisecond

type taskerWatcher struct {
	watcher *fsnotify.Watcher
	root    string
	send    func(tea.Msg)

	mu    sync.Mutex
	timer *time.Timer
}

func startTaskerWatcher(root string, send func(tea.Msg)) (*taskerWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	taskerWatcher := &taskerWatcher{
		watcher: watcher,
		root:    root,
		send:    send,
	}
	if err := taskerWatcher.sync(); err != nil {
		watcher.Close()
		return nil, err
	}

	go taskerWatcher.run()
	return taskerWatcher, nil
}

func (w *taskerWatcher) Close() error {
	w.mu.Lock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.mu.Unlock()
	return w.watcher.Close()
}

func (w *taskerWatcher) run() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op == fsnotify.Chmod {
				continue
			}
			if err := w.syncForEvent(event.Name); err != nil {
				w.send(fileWatchErrorMsg{Err: err})
				continue
			}
			w.scheduleRefresh()
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.send(fileWatchErrorMsg{Err: err})
		}
	}
}

func (w *taskerWatcher) scheduleRefresh() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(fileWatchDebounce, func() {
		w.send(fileChangeMsg{})
	})
}

func (w *taskerWatcher) syncForEvent(path string) error {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return addWatchTree(w.watcher, path)
	}
	if os.IsNotExist(err) {
		return w.sync()
	}
	if err != nil {
		return err
	}
	return nil
}

func (w *taskerWatcher) sync() error {
	dirs, err := listTaskerWatchDirs(w.root)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := w.watcher.Add(dir); err != nil {
			var pathErr *os.PathError
			if os.IsNotExist(err) || (errors.As(err, &pathErr) && os.IsNotExist(pathErr.Err)) {
				continue
			}
			return err
		}
	}
	return nil
}

func listTaskerWatchDirs(root string) ([]string, error) {
	taskerRoot := filepath.Join(root, tasker.TaskerDirName)
	return walkDirs(taskerRoot)
}

func addWatchTree(watcher *fsnotify.Watcher, root string) error {
	dirs, err := walkDirs(root)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := watcher.Add(dir); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func walkDirs(root string) ([]string, error) {
	dirs := make([]string, 0, 32)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	return dirs, nil
}
