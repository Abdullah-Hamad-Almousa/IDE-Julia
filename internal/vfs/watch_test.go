package vfs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// waitEvent waits for a file-system event that matches the predicate.
func waitEvent(t *testing.T, events <-chan WatchEvent, timeout time.Duration, match func(WatchEvent) bool) WatchEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("the event channel was closed")
			}
			if match(event) {
				return event
			}
		case <-deadline:
			t.Fatal("the expected file-system event was not delivered")
		}
	}
}

// TestPollingWatcherDetectsChanges checks that the polling backend reports
// the creation, the modification and the removal of a file.
func TestPollingWatcherDetectsChanges(t *testing.T) {
	dir := t.TempDir()
	clock := newFakeClock()
	watcher := NewPollingWatcherWithClock(20*time.Millisecond, clock)
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	backend, ok := watcher.backend.(*pollingBackend)
	if !ok {
		t.Fatalf("the watcher backend is %T, want *pollingBackend", watcher.backend)
	}

	// The ticker of the backend is created in the goroutine of the backend,
	// so the test waits for it before it moves the clock.
	waitFor(t, 2*time.Second, func() bool { return clock.tickerCount() > 0 })
	if err := watcher.AddRoot(dir); err != nil {
		t.Fatalf("AddRoot() error = %v", err)
	}

	// The first scan establishes the baseline of the backend.
	clock.Advance(20 * time.Millisecond)
	waitFor(t, 2*time.Second, func() bool { return backend.pollCount() > 0 })

	created := writeFile(t, dir, "a.jl", "x = 1\n")
	clock.Advance(20 * time.Millisecond)
	event := waitEvent(t, watcher.Events(), 2*time.Second, func(e WatchEvent) bool {
		return e.Path == created && e.Op.Has(OpCreate)
	})
	if !event.Op.Has(OpCreate) {
		t.Errorf("event op = %q, want a create", event.Op)
	}

	// The content grows, so the change is visible even when the file system
	// reports the same modification time for the two writes.
	if err := os.WriteFile(created, []byte("x = 22\n"), 0o644); err != nil {
		t.Fatalf("write %q: %v", created, err)
	}
	clock.Advance(20 * time.Millisecond)
	waitEvent(t, watcher.Events(), 2*time.Second, func(e WatchEvent) bool {
		return e.Path == created && e.Op.Has(OpWrite)
	})

	if err := os.Remove(created); err != nil {
		t.Fatalf("remove %q: %v", created, err)
	}
	clock.Advance(20 * time.Millisecond)
	waitEvent(t, watcher.Events(), 2*time.Second, func(e WatchEvent) bool {
		return e.Path == created && e.Op.Has(OpRemove)
	})
}

// TestPollingWatcherRemoveRoot checks that a tree that is not watched cannot
// be removed.
func TestPollingWatcherRemoveRoot(t *testing.T) {
	watcher := NewPollingWatcherWithClock(20*time.Millisecond, newFakeClock())
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	if err := watcher.RemoveRoot(t.TempDir()); err == nil {
		t.Error("RemoveRoot() of a tree that is not watched must fail")
	}
}

// TestFSNotifyWatcherDetectsCreate checks that the native backend reports the
// creation of a file.
//
// The kqueue backend of the BSD systems does not report the modification of
// the files of a watched directory, so only the creation is asserted.
func TestFSNotifyWatcherDetectsCreate(t *testing.T) {
	watcher, err := NewFSNotifyWatcher()
	if err != nil {
		t.Skipf("the native file-system notifications are not available: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	dir := t.TempDir()
	if err := watcher.AddRoot(dir); err != nil {
		t.Fatalf("AddRoot() error = %v", err)
	}

	created := writeFile(t, dir, "a.jl", "x = 1\n")
	event := waitEvent(t, watcher.Events(), 10*time.Second, func(e WatchEvent) bool {
		return e.Path == created
	})
	if !event.Op.Has(OpCreate) && !event.Op.Has(OpWrite) {
		t.Errorf("event op = %q, want a create or a write", event.Op)
	}
}

// TestFSNotifyWatcherDetectsRemove checks that the native backend reports the
// removal of a file.
func TestFSNotifyWatcherDetectsRemove(t *testing.T) {
	watcher, err := NewFSNotifyWatcher()
	if err != nil {
		t.Skipf("the native file-system notifications are not available: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	dir := t.TempDir()
	removed := writeFile(t, dir, "a.jl", "x = 1\n")
	if err := watcher.AddRoot(dir); err != nil {
		t.Fatalf("AddRoot() error = %v", err)
	}
	if err := os.Remove(removed); err != nil {
		t.Fatalf("remove %q: %v", removed, err)
	}
	event := waitEvent(t, watcher.Events(), 10*time.Second, func(e WatchEvent) bool {
		return e.Path == removed && (e.Op.Has(OpRemove) || e.Op.Has(OpRename))
	})
	if !event.Op.Has(OpRemove) && !event.Op.Has(OpRename) {
		t.Errorf("event op = %q, want a remove or a rename", event.Op)
	}
}

// TestWatcherCloseClosesTheChannels checks that the watcher releases its
// resources.
func TestWatcherCloseClosesTheChannels(t *testing.T) {
	watcher := NewPollingWatcherWithClock(20*time.Millisecond, newFakeClock())
	if err := watcher.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case _, ok := <-watcher.Events():
		if ok {
			t.Error("the event channel must be closed after Close()")
		}
	case <-time.After(2 * time.Second):
		t.Error("the event channel was not closed after Close()")
	}
}

// TestOpString checks the textual form of the operation masks.
func TestOpString(t *testing.T) {
	tests := []struct {
		op   Op
		want string
	}{
		{Op(0), "none"},
		{OpCreate, "create"},
		{OpWrite, "write"},
		{OpRemove, "remove"},
		{OpRename, "rename"},
		{OpChmod, "chmod"},
		{OpCreate | OpWrite, "create|write"},
	}
	for _, test := range tests {
		if got := test.op.String(); got != test.want {
			t.Errorf("Op(%d).String() = %q, want %q", test.op, got, test.want)
		}
	}
	if !OpCreate.Has(OpCreate) {
		t.Error("OpCreate must contain OpCreate")
	}
	if OpCreate.Has(OpWrite) {
		t.Error("OpCreate must not contain OpWrite")
	}
}

// TestWatcherBackendIsSwappable checks that the watcher works with a backend
// that is not the native one.
func TestWatcherBackendIsSwappable(t *testing.T) {
	backend := &recordingBackend{
		events: make(chan WatchEvent, 1),
		errors: make(chan error, 1),
	}
	watcher := NewWatcher(backend)
	if err := watcher.AddRoot(t.TempDir()); err != nil {
		t.Fatalf("AddRoot() error = %v", err)
	}
	backend.events <- WatchEvent{Path: filepath.Join("dir", "a.jl"), Op: OpWrite}
	event := waitEvent(t, watcher.Events(), 2*time.Second, func(e WatchEvent) bool {
		return e.Op.Has(OpWrite)
	})
	if got, want := event.Op, OpWrite; got != want {
		t.Errorf("event op = %q, want %q", got, want)
	}
	if err := watcher.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !backend.closed {
		t.Error("Close() must close the backend")
	}
}

// recordingBackend is a backend that records the calls of the watcher.
type recordingBackend struct {
	events chan WatchEvent
	errors chan error
	closed bool
}

// AddRoot records the watched tree.
func (b *recordingBackend) AddRoot(string) error { return nil }

// RemoveRoot records the removed tree.
func (b *recordingBackend) RemoveRoot(string) error { return nil }

// Events returns the channel of the backend.
func (b *recordingBackend) Events() <-chan WatchEvent { return b.events }

// Errors returns the channel of the backend.
func (b *recordingBackend) Errors() <-chan error { return b.errors }

// Close closes the backend.
func (b *recordingBackend) Close() error {
	b.closed = true
	return nil
}

// TestNativeBackendIsUsedOnThisPlatform documents the backend the platform
// provides. The test only fails when the native backend cannot be created on
// a platform that is known to support it.
func TestNativeBackendIsUsedOnThisPlatform(t *testing.T) {
	watcher, err := NewFSNotifyWatcher()
	if err != nil {
		if runtime.GOOS == "linux" || runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			t.Fatalf("NewFSNotifyWatcher() error = %v", err)
		}
		t.Skipf("the native file-system notifications are not available: %v", err)
	}
	if err := watcher.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}
