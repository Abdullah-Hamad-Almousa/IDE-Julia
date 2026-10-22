package vfs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDebouncerCoalescesABurst checks that a burst of notifications produces
// exactly one change event.
func TestDebouncerCoalescesABurst(t *testing.T) {
	clock := newFakeClock()
	debouncer := NewDebouncer(50*time.Millisecond, clock)
	defer func() {
		if err := debouncer.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	debouncer.Notify("/tmp/a.jl")
	debouncer.Notify("/tmp/b.jl")
	debouncer.Notify("/tmp/a.jl")

	select {
	case change := <-debouncer.Changes():
		t.Fatalf("the change was emitted before the interval elapsed: %v", change.Paths)
	default:
	}

	clock.Advance(50 * time.Millisecond)

	select {
	case change := <-debouncer.Changes():
		if len(change.Paths) != 2 {
			t.Fatalf("change paths = %v, want two paths", change.Paths)
		}
		if change.Paths[0] != "/tmp/a.jl" || change.Paths[1] != "/tmp/b.jl" {
			t.Errorf("change paths = %v, want [/tmp/a.jl /tmp/b.jl]", change.Paths)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the change was not emitted")
	}

	select {
	case change := <-debouncer.Changes():
		t.Errorf("an extra change was emitted: %v", change.Paths)
	default:
	}
}

// TestDebouncerRestartsTheTimer checks that every notification postpones the
// emission of the change.
func TestDebouncerRestartsTheTimer(t *testing.T) {
	clock := newFakeClock()
	debouncer := NewDebouncer(50*time.Millisecond, clock)
	defer func() {
		if err := debouncer.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	debouncer.Notify("/tmp/a.jl")
	clock.Advance(40 * time.Millisecond)
	debouncer.Notify("/tmp/b.jl")
	clock.Advance(40 * time.Millisecond)

	select {
	case change := <-debouncer.Changes():
		t.Fatalf("the change was emitted before the burst was over: %v", change.Paths)
	default:
	}

	clock.Advance(20 * time.Millisecond)
	select {
	case change := <-debouncer.Changes():
		if len(change.Paths) != 2 {
			t.Fatalf("change paths = %v, want two paths", change.Paths)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the change was not emitted")
	}
}

// TestDebouncerIntervalIsAParameter checks that the debounce interval is not
// a constant of the package.
func TestDebouncerIntervalIsAParameter(t *testing.T) {
	debouncer := NewDebouncer(250*time.Millisecond, newFakeClock())
	defer func() {
		if err := debouncer.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()
	if got, want := debouncer.Interval(), 250*time.Millisecond; got != want {
		t.Errorf("interval = %s, want %s", got, want)
	}
}

// TestPipelineCoalescesTheStoreEvents checks that the save pipeline turns a
// burst of edits into a single notification.
func TestPipelineCoalescesTheStoreEvents(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	clock := newFakeClock()
	store := NewStore()
	pipeline := NewPipeline(store, 50*time.Millisecond, clock)
	defer func() {
		if err := pipeline.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := file.Write([]byte("x = 2\n")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if _, err := file.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// The pipeline forwards the events of the store in its own goroutine, so
	// the test waits until the debouncer knows the path.
	waitFor(t, 2*time.Second, func() bool {
		clock.Advance(time.Millisecond)
		select {
		case change := <-pipeline.Changes():
			if len(change.Paths) != 1 || change.Paths[0] != path {
				t.Errorf("change paths = %v, want [%s]", change.Paths, path)
			}
			return true
		default:
			return false
		}
	})
}

// TestPipelineIgnoresTheOpenAndCloseEvents checks that the pipeline only
// reports the changes of the content.
func TestPipelineIgnoresTheOpenAndCloseEvents(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	clock := newFakeClock()
	store := NewStore()
	pipeline := NewPipeline(store, 50*time.Millisecond, clock)
	defer func() {
		if err := pipeline.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	if _, err := store.Open(path); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	clock.Advance(time.Second)

	select {
	case change := <-pipeline.Changes():
		t.Errorf("the pipeline reported the open of a file: %v", change.Paths)
	default:
	}
}

// TestStoreSubscribeDeliversEveryEvent checks that a slow consumer does not
// lose events.
func TestStoreSubscribeDeliversEveryEvent(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	store := NewStore()
	subscription := store.Subscribe()
	defer subscription.Close()

	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := file.Write([]byte("x = 2\n")); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	want := 11 // one open event and ten write events
	for i := 0; i < want; i++ {
		select {
		case <-subscription.Events():
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of %d events were delivered", i, want)
		}
	}
}

// TestSubscriptionCloseClosesTheChannel checks the shutdown of a
// subscription.
func TestSubscriptionCloseClosesTheChannel(t *testing.T) {
	store := NewStore()
	subscription := store.Subscribe()
	subscription.Close()
	subscription.Close()

	select {
	case _, ok := <-subscription.Events():
		if ok {
			t.Error("the event channel must be closed")
		}
	case <-time.After(2 * time.Second):
		t.Error("the event channel was not closed")
	}
}

// TestStorePathsAreSorted checks the order of the open files.
func TestStorePathsAreSorted(t *testing.T) {
	dir := t.TempDir()
	first := writeFile(t, dir, "a.jl", "")
	second := writeFile(t, dir, "b.jl", "")

	store := NewStore()
	if _, err := store.Open(second); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := store.Open(first); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	paths := store.Paths()
	if len(paths) != 2 {
		t.Fatalf("store paths = %v, want two paths", paths)
	}
	if paths[0] > paths[1] {
		t.Errorf("store paths = %v, want a sorted slice", paths)
	}
	if _, ok := store.Snapshot(filepath.Join(dir, "a.jl")); !ok {
		t.Error("Snapshot() did not find the open file")
	}
	if _, ok := store.Snapshot(filepath.Join(dir, "missing.jl")); ok {
		t.Error("Snapshot() found a file that is not open")
	}
}

// TestStoreSaveWritesTheFile checks that the saved file holds the content of
// the last write.
func TestStoreSaveWritesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "")

	store := NewStore()
	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := file.Write([]byte("x = 42\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := file.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(content), "x = 42\n"; got != want {
		t.Errorf("content on disk = %q, want %q", got, want)
	}
}
