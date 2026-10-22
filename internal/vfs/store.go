package vfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

// Errors returned by the store.
var (
	// ErrNotOpen is returned when a file that is not open in the store is
	// accessed.
	ErrNotOpen = errors.New("vfs: the file is not open")
	// ErrNoSuchFile is returned when a file does not exist on disk.
	ErrNoSuchFile = errors.New("vfs: the file does not exist")
)

// EventKind classifies a document event.
type EventKind string

const (
	// EventOpen is emitted when a file is opened in the store.
	EventOpen EventKind = "open"
	// EventWrite is emitted when the in-memory content of a file changes.
	EventWrite EventKind = "write"
	// EventSave is emitted when a file is written to disk.
	EventSave EventKind = "save"
	// EventClose is emitted when a file is closed in the store.
	EventClose EventKind = "close"
)

// Event describes a change of a file that is open in a store.
type Event struct {
	// Kind classifies the event.
	Kind EventKind
	// Path is the absolute path of the file.
	Path string
	// Snapshot is the state of the file after the event.
	Snapshot Snapshot
}

// eventBufferSize is the capacity of the channel of a subscription. The
// subscription owns an unbounded queue in front of the channel, so the buffer
// only smooths bursts; it never drops an event.
const eventBufferSize = 64

// firstVersion is the version of a file that was read from disk. Every write
// and every save produces a greater version.
const firstVersion = 1

// Store keeps the files that are open in an engine session together with
// their snapshots.
//
// The store is safe for concurrent use.
type Store struct {
	mutex sync.RWMutex
	files map[string]*fileState
	subs  map[*Subscription]struct{}
}

// fileState is the mutable state of an open file.
type fileState struct {
	version int64
	content []byte
	modTime time.Time
	dirty   bool
}

// snapshot returns the immutable view of the state.
func (s *fileState) snapshot(path string) Snapshot {
	snapshot := NewSnapshot(path, s.version, s.content, s.modTime)
	snapshot.dirty = s.dirty
	return snapshot
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{
		files: make(map[string]*fileState),
		subs:  make(map[*Subscription]struct{}),
	}
}

// Open opens the file at path and returns a handle to it.
//
// The file is read from disk when it is opened for the first time. Opening a
// file that is already open returns a handle to the same state, so the
// in-memory content is not lost.
func (s *Store) Open(path string) (*File, error) {
	absolute, err := absolutePath(path)
	if err != nil {
		return nil, err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if _, ok := s.files[absolute]; ok {
		return &File{store: s, path: absolute}, nil
	}

	content, err := os.ReadFile(absolute)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNoSuchFile, absolute)
		}
		return nil, fmt.Errorf("vfs: read %q: %w", absolute, err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("vfs: stat %q: %w", absolute, err)
	}

	s.files[absolute] = &fileState{
		version: firstVersion,
		content: content,
		modTime: info.ModTime(),
	}
	state := s.files[absolute]
	s.notify(Event{Kind: EventOpen, Path: absolute, Snapshot: state.snapshot(absolute)})
	return &File{store: s, path: absolute}, nil
}

// Close closes the file at path and drops its state.
func (s *Store) Close(path string) error {
	absolute, err := absolutePath(path)
	if err != nil {
		return err
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	state, ok := s.files[absolute]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNotOpen, absolute)
	}
	delete(s.files, absolute)
	s.notify(Event{Kind: EventClose, Path: absolute, Snapshot: state.snapshot(absolute)})
	return nil
}

// Paths returns the absolute paths of the open files, sorted.
func (s *Store) Paths() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	paths := make([]string, 0, len(s.files))
	for path := range s.files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
}

// Snapshot returns the current snapshot of the file at path.
func (s *Store) Snapshot(path string) (Snapshot, bool) {
	absolute, err := absolutePath(path)
	if err != nil {
		return Snapshot{}, false
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	state, ok := s.files[absolute]
	if !ok {
		return Snapshot{}, false
	}
	return state.snapshot(absolute), true
}

// Subscribe returns a subscription that receives the events of the store.
//
// The caller must close the subscription when it is no longer needed.
func (s *Store) Subscribe() *Subscription {
	subscription := &Subscription{
		store:   s,
		events:  make(chan Event, eventBufferSize),
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}

	s.mutex.Lock()
	s.subs[subscription] = struct{}{}
	s.mutex.Unlock()

	go subscription.run()
	return subscription
}

// notify publishes an event to every subscription of the store.
//
// The caller must hold the lock of the store.
func (s *Store) notify(event Event) {
	for subscription := range s.subs {
		subscription.push(event)
	}
}

// removeSubscription drops a subscription from the store.
func (s *Store) removeSubscription(subscription *Subscription) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.subs, subscription)
}

// File is a handle to a file that is open in a store.
type File struct {
	store *Store
	path  string
}

// Path returns the absolute path of the file.
func (f *File) Path() string { return f.path }

// Read returns the current snapshot of the file.
//
// The snapshot holds the in-memory content of the file, which may differ
// from the file on disk.
func (f *File) Read() (Snapshot, error) {
	f.store.mutex.RLock()
	defer f.store.mutex.RUnlock()

	state, ok := f.store.files[f.path]
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrNotOpen, f.path)
	}
	return state.snapshot(f.path), nil
}

// Write replaces the in-memory content of the file.
//
// The write produces a new revision of the file: the version of the returned
// snapshot is greater than the version of the previous snapshot. The file is
// marked as dirty until it is saved.
func (f *File) Write(content []byte) (Snapshot, error) {
	f.store.mutex.Lock()
	defer f.store.mutex.Unlock()

	state, ok := f.store.files[f.path]
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrNotOpen, f.path)
	}
	buffer := make([]byte, len(content))
	copy(buffer, content)

	state.content = buffer
	state.version++
	state.dirty = true

	snapshot := state.snapshot(f.path)
	f.store.notify(Event{Kind: EventWrite, Path: f.path, Snapshot: snapshot})
	return snapshot, nil
}

// Save writes the in-memory content of the file to disk.
//
// The save produces a new revision of the file: the version of the returned
// snapshot is greater than the version of the previous snapshot. The
// subscribers of the store receive a save event.
func (f *File) Save() (Snapshot, error) {
	f.store.mutex.Lock()
	defer f.store.mutex.Unlock()

	state, ok := f.store.files[f.path]
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: %s", ErrNotOpen, f.path)
	}
	if err := os.WriteFile(f.path, state.content, 0o644); err != nil {
		return Snapshot{}, fmt.Errorf("vfs: write %q: %w", f.path, err)
	}
	info, err := os.Stat(f.path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("vfs: stat %q: %w", f.path, err)
	}

	state.modTime = info.ModTime()
	state.version++
	state.dirty = false

	snapshot := state.snapshot(f.path)
	f.store.notify(Event{Kind: EventSave, Path: f.path, Snapshot: snapshot})
	return snapshot, nil
}

// Subscription delivers the events of a store.
//
// The events are queued without a limit, so a slow consumer never loses an
// event. Close the subscription to stop the delivery.
type Subscription struct {
	store  *Store
	events chan Event

	mutex   sync.Mutex
	pending []Event

	wake      chan struct{}
	done      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
}

// Events returns the channel the events are delivered on.
//
// The channel is closed when the subscription is closed.
func (s *Subscription) Events() <-chan Event { return s.events }

// Done returns the channel that is closed when the subscription is closed.
func (s *Subscription) Done() <-chan struct{} { return s.done }

// Close stops the delivery of events and closes the event channel.
func (s *Subscription) Close() {
	s.closeOnce.Do(func() {
		s.store.removeSubscription(s)
		close(s.done)
		<-s.stopped
		close(s.events)
	})
}

// push queues an event for the subscription.
func (s *Subscription) push(event Event) {
	s.mutex.Lock()
	s.pending = append(s.pending, event)
	s.mutex.Unlock()

	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run moves the queued events to the event channel.
func (s *Subscription) run() {
	defer close(s.stopped)
	for {
		s.mutex.Lock()
		pending := s.pending
		s.pending = nil
		s.mutex.Unlock()

		for _, event := range pending {
			select {
			case s.events <- event:
			case <-s.done:
				return
			}
		}

		select {
		case <-s.wake:
		case <-s.done:
			return
		}
	}
}

// absolutePath returns the cleaned absolute form of path.
func absolutePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("vfs: %w: the path is empty", ErrNoSuchFile)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("vfs: resolve %q: %w", path, err)
	}
	return filepath.Clean(absolute), nil
}
