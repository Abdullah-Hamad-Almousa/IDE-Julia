package vfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// pollingBackend discovers the file-system changes by comparing the state of
// the watched trees with the state of the previous scan.
//
// The backend is the fallback for the platforms where the native
// notifications are not available. It cannot detect a rename: a renamed file
// is reported as the removal of the old path and the creation of the new one.
type pollingBackend struct {
	clock    Clock
	interval time.Duration
	events   chan WatchEvent
	errors   chan error
	done     chan struct{}

	mutex sync.Mutex
	roots map[string]bool
	known map[string]map[string]polledFile
	polls int

	closeOnce sync.Once
}

// polledFile is the state of a file that the polling backend knows about.
type polledFile struct {
	size    int64
	modTime time.Time
}

// newPollingBackend returns a backend that polls the watched trees.
func newPollingBackend(interval time.Duration, clock Clock) *pollingBackend {
	if interval <= 0 {
		interval = time.Second
	}
	backend := &pollingBackend{
		clock:    clock,
		interval: interval,
		events:   make(chan WatchEvent, eventBufferSize),
		errors:   make(chan error, eventBufferSize),
		done:     make(chan struct{}),
		roots:    make(map[string]bool),
		known:    make(map[string]map[string]polledFile),
	}
	go backend.run()
	return backend
}

// AddRoot starts watching the directory tree rooted at path.
func (b *pollingBackend) AddRoot(path string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("vfs: watch %q: %w", path, err)
	}
	b.roots[path] = true
	return nil
}

// RemoveRoot stops watching the directory tree rooted at path.
func (b *pollingBackend) RemoveRoot(path string) error {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	if !b.roots[path] {
		return fmt.Errorf("vfs: the tree %q is not watched", path)
	}
	delete(b.roots, path)
	delete(b.known, path)
	return nil
}

// Events returns the channel the file-system events are delivered on.
func (b *pollingBackend) Events() <-chan WatchEvent { return b.events }

// Errors returns the channel the backend errors are delivered on.
func (b *pollingBackend) Errors() <-chan error { return b.errors }

// Close releases the resources of the backend.
func (b *pollingBackend) Close() error {
	b.closeOnce.Do(func() {
		close(b.done)
	})
	return nil
}

// run polls the watched trees until the backend is closed.
func (b *pollingBackend) run() {
	defer close(b.events)
	defer close(b.errors)

	ticker := b.clock.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-b.done:
			return
		case <-ticker.C():
			b.poll()
		}
	}
}

// poll scans the watched trees once and reports the differences.
func (b *pollingBackend) poll() {
	b.mutex.Lock()
	roots := make([]string, 0, len(b.roots))
	for root := range b.roots {
		roots = append(roots, root)
	}
	b.mutex.Unlock()

	for _, root := range roots {
		b.pollRoot(root)
	}
}

// pollRoot scans one tree and reports the differences.
func (b *pollingBackend) pollRoot(root string) {
	seen := make(map[string]polledFile)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory must not abort the scan, so the
			// directory is skipped.
			return fs.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, statErr := entry.Info()
		if statErr == nil {
			seen[path] = polledFile{size: info.Size(), modTime: info.ModTime()}
		}
		return nil
	})
	if err != nil {
		b.report(fmt.Errorf("vfs: poll %q: %w", root, err))
		return
	}

	b.mutex.Lock()
	previous := b.known[root]
	b.known[root] = seen
	b.polls++
	b.mutex.Unlock()

	for path, state := range seen {
		old, existed := previous[path]
		switch {
		case !existed:
			b.send(WatchEvent{Path: path, Op: OpCreate})
		case old.size != state.size || !old.modTime.Equal(state.modTime):
			b.send(WatchEvent{Path: path, Op: OpWrite})
		}
	}
	for path := range previous {
		if _, ok := seen[path]; !ok {
			b.send(WatchEvent{Path: path, Op: OpRemove})
		}
	}
}

// send publishes an event.
func (b *pollingBackend) send(event WatchEvent) {
	select {
	case b.events <- event:
	case <-b.done:
	}
}

// polls returns the number of completed scans. The tests use it to wait for
// the baseline of the polling backend.
func (b *pollingBackend) pollCount() int {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.polls
}

// report publishes an error.
func (b *pollingBackend) report(err error) {
	select {
	case b.errors <- err:
	case <-b.done:
	}
}
