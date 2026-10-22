package vfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// fsnotifyBackend publishes the file-system events of the operating system.
//
// The native notifications of the operating system are not recursive, so the
// backend watches every directory of a tree on its own and adds the new
// directories when they appear.
type fsnotifyBackend struct {
	watcher *fsnotify.Watcher
	events  chan WatchEvent
	errors  chan error
	done    chan struct{}

	mutex sync.Mutex
	roots map[string]bool

	closeOnce sync.Once
}

// newFSNotifyBackend returns a backend that uses the native file-system
// notifications.
func newFSNotifyBackend() (*fsnotifyBackend, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("vfs: create the fsnotify watcher: %w", err)
	}
	backend := &fsnotifyBackend{
		watcher: watcher,
		events:  make(chan WatchEvent, eventBufferSize),
		errors:  make(chan error, eventBufferSize),
		done:    make(chan struct{}),
		roots:   make(map[string]bool),
	}
	go backend.run()
	return backend, nil
}

// AddRoot starts watching the directory tree rooted at path.
func (b *fsnotifyBackend) AddRoot(path string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if err := b.addTree(path); err != nil {
		return err
	}
	b.roots[path] = true
	return nil
}

// RemoveRoot stops watching the directory tree rooted at path.
//
// The directories of the tree stay watched until the backend is closed: the
// native notifications do not offer a cheap way to enumerate them again.
func (b *fsnotifyBackend) RemoveRoot(path string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.roots[path] {
		return fmt.Errorf("vfs: the tree %q is not watched", path)
	}
	delete(b.roots, path)
	return nil
}

// Events returns the channel the file-system events are delivered on.
func (b *fsnotifyBackend) Events() <-chan WatchEvent { return b.events }

// Errors returns the channel the backend errors are delivered on.
func (b *fsnotifyBackend) Errors() <-chan error { return b.errors }

// Close releases the resources of the backend.
func (b *fsnotifyBackend) Close() error {
	var err error
	b.closeOnce.Do(func() {
		close(b.done)
		err = b.watcher.Close()
	})
	return err
}

// addTree watches every directory of the tree rooted at path.
//
// The caller must hold the lock of the backend.
func (b *fsnotifyBackend) addTree(path string) error {
	return filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory must not abort the whole watch, so
			// the directory is skipped.
			return fs.SkipDir
		}
		if !entry.IsDir() {
			return nil
		}
		if addErr := b.watcher.Add(current); addErr != nil {
			return fmt.Errorf("vfs: watch %q: %w", current, addErr)
		}
		return nil
	})
}

// run translates the native events into the events of the backend.
func (b *fsnotifyBackend) run() {
	defer close(b.events)
	defer close(b.errors)

	for {
		select {
		case <-b.done:
			return
		case event, ok := <-b.watcher.Events:
			if !ok {
				return
			}
			b.handle(event)
		case err, ok := <-b.watcher.Errors:
			if !ok {
				return
			}
			select {
			case b.errors <- err:
			case <-b.done:
				return
			}
		}
	}
}

// handle translates a single native event.
func (b *fsnotifyBackend) handle(event fsnotify.Event) {
	op := Op(0)
	switch {
	case event.Has(fsnotify.Create):
		op |= OpCreate
	case event.Has(fsnotify.Write):
		op |= OpWrite
	case event.Has(fsnotify.Remove):
		op |= OpRemove
	case event.Has(fsnotify.Rename):
		op |= OpRename
	case event.Has(fsnotify.Chmod):
		op |= OpChmod
	}
	if op == 0 {
		return
	}

	// A new directory must be watched as well, otherwise its content stays
	// invisible to the watcher.
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			b.mutex.Lock()
			_ = b.addTree(event.Name)
			b.mutex.Unlock()
		}
	}

	select {
	case b.events <- WatchEvent{Path: event.Name, Op: op}:
	case <-b.done:
	}
}
