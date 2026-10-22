package vfs

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Op is a bit mask of file-system operations.
type Op uint32

const (
	// OpCreate reports that a file or a directory appeared.
	OpCreate Op = 1 << iota
	// OpWrite reports that the content of a file changed.
	OpWrite
	// OpRemove reports that a file or a directory disappeared.
	OpRemove
	// OpRename reports that a file or a directory was renamed.
	OpRename
	// OpChmod reports that the permissions of a file changed.
	OpChmod
)

// Has reports whether the operation mask contains op.
func (o Op) Has(op Op) bool { return o&op != 0 }

// String returns a stable, human readable form of the operation mask.
func (o Op) String() string {
	if o == 0 {
		return "none"
	}
	names := make([]string, 0, 5)
	for _, entry := range []struct {
		op   Op
		name string
	}{
		{OpCreate, "create"},
		{OpWrite, "write"},
		{OpRemove, "remove"},
		{OpRename, "rename"},
		{OpChmod, "chmod"},
	} {
		if o.Has(entry.op) {
			names = append(names, entry.name)
		}
	}
	return strings.Join(names, "|")
}

// WatchEvent is a file-system event of a watched tree.
type WatchEvent struct {
	// Path is the absolute path of the file the event is about.
	Path string
	// Op is the operation mask of the event.
	Op Op
}

// Backend is a pluggable source of file-system events.
//
// The native fsnotify backend and the polling backend implement it, so the
// watcher can be exercised headlessly and the backend can be swapped without
// touching the watcher.
type Backend interface {
	// AddRoot starts watching the directory tree rooted at path.
	AddRoot(path string) error
	// RemoveRoot stops watching the directory tree rooted at path.
	RemoveRoot(path string) error
	// Events returns the channel the file-system events are delivered on.
	Events() <-chan WatchEvent
	// Errors returns the channel the backend errors are delivered on.
	Errors() <-chan error
	// Close releases the resources of the backend.
	Close() error
}

// Watcher publishes the file-system events of the watched directory trees.
//
// The watcher owns the backend and forwards its events and errors to the
// channels returned by Events and Errors.
type Watcher struct {
	backend Backend
	events  chan WatchEvent
	errors  chan error
	done    chan struct{}

	closeOnce sync.Once
	closeErr  error
}

// NewWatcher returns a watcher that publishes the events of the backend.
func NewWatcher(backend Backend) *Watcher {
	watcher := &Watcher{
		backend: backend,
		events:  make(chan WatchEvent, eventBufferSize),
		errors:  make(chan error, eventBufferSize),
		done:    make(chan struct{}),
	}
	go watcher.pump()
	return watcher
}

// NewFSNotifyWatcher returns a watcher backed by the native file-system
// notifications of the operating system.
func NewFSNotifyWatcher() (*Watcher, error) {
	backend, err := newFSNotifyBackend()
	if err != nil {
		return nil, err
	}
	return NewWatcher(backend), nil
}

// NewPollingWatcher returns a watcher that polls the watched trees.
//
// The polling backend is the fallback for the platforms where the native
// notifications are not available or not reliable.
func NewPollingWatcher(interval time.Duration) *Watcher {
	return NewPollingWatcherWithClock(interval, SystemClock{})
}

// NewPollingWatcherWithClock returns a polling watcher that is driven by the
// given clock.
//
// The clock makes the polling backend testable without sleeping.
func NewPollingWatcherWithClock(interval time.Duration, clock Clock) *Watcher {
	return NewWatcher(newPollingBackend(interval, clock))
}

// AddRoot starts watching the directory tree rooted at path.
func (w *Watcher) AddRoot(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("vfs: resolve %q: %w", path, err)
	}
	return w.backend.AddRoot(filepath.Clean(absolute))
}

// RemoveRoot stops watching the directory tree rooted at path.
func (w *Watcher) RemoveRoot(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("vfs: resolve %q: %w", path, err)
	}
	return w.backend.RemoveRoot(filepath.Clean(absolute))
}

// Events returns the channel the file-system events are delivered on.
//
// The channel is closed when the watcher is closed.
func (w *Watcher) Events() <-chan WatchEvent { return w.events }

// Errors returns the channel the backend errors are delivered on.
//
// The channel is closed when the watcher is closed.
func (w *Watcher) Errors() <-chan error { return w.errors }

// Close stops the watcher and releases the resources of the backend.
func (w *Watcher) Close() error {
	w.closeOnce.Do(func() {
		close(w.done)
		w.closeErr = w.backend.Close()
	})
	return w.closeErr
}

// pump forwards the events and the errors of the backend.
func (w *Watcher) pump() {
	defer close(w.events)
	defer close(w.errors)

	backendEvents := w.backend.Events()
	backendErrors := w.backend.Errors()
	for {
		select {
		case <-w.done:
			return
		case event, ok := <-backendEvents:
			if !ok {
				return
			}
			select {
			case w.events <- event:
			case <-w.done:
				return
			}
		case err, ok := <-backendErrors:
			if !ok {
				backendErrors = nil
				continue
			}
			select {
			case w.errors <- err:
			case <-w.done:
				return
			}
		}
	}
}
