package vfs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitFor waits until the condition holds or the timeout expires.
func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the condition was not met in time")
}

// writeFile writes a file in a directory and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	return path
}

// TestSnapshotIsImmutable checks that a snapshot does not change when its
// content is modified.
func TestSnapshotIsImmutable(t *testing.T) {
	content := []byte("x = 1\n")
	snapshot := NewSnapshot("/tmp/a.jl", 1, content, time.Unix(0, 0))

	content[0] = 'y'
	if got, want := snapshot.String(), "x = 1\n"; got != want {
		t.Errorf("snapshot content = %q, want %q", got, want)
	}

	buffer := snapshot.Bytes()
	buffer[0] = 'z'
	if got, want := snapshot.String(), "x = 1\n"; got != want {
		t.Errorf("snapshot content = %q after the copy was modified, want %q", got, want)
	}
	if got, want := snapshot.Size(), int64(6); got != want {
		t.Errorf("snapshot size = %d, want %d", got, want)
	}
	if got, want := snapshot.Version(), int64(1); got != want {
		t.Errorf("snapshot version = %d, want %d", got, want)
	}
	if got, want := snapshot.Path(), "/tmp/a.jl"; got != want {
		t.Errorf("snapshot path = %q, want %q", got, want)
	}
	if snapshot.Dirty() {
		t.Error("a fresh snapshot must not be dirty")
	}
}

// TestStoreOpenReadWriteSave checks the read, write and save semantics of the
// document store.
func TestStoreOpenReadWriteSave(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	store := NewStore()
	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got, want := file.Path(), path; got != want {
		t.Errorf("file path = %q, want %q", got, want)
	}

	snapshot, err := file.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got, want := snapshot.Version(), int64(1); got != want {
		t.Errorf("version after Open = %d, want %d", got, want)
	}
	if got, want := snapshot.String(), "x = 1\n"; got != want {
		t.Errorf("content after Open = %q, want %q", got, want)
	}
	if snapshot.Dirty() {
		t.Error("a file that was read from disk must not be dirty")
	}

	written, err := file.Write([]byte("x = 2\n"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got, want := written.Version(), int64(2); got != want {
		t.Errorf("version after Write = %d, want %d", got, want)
	}
	if !written.Dirty() {
		t.Error("a file that was written must be dirty")
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(onDisk), "x = 1\n"; got != want {
		t.Errorf("content on disk = %q, want %q", got, want)
	}

	saved, err := file.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if got, want := saved.Version(), int64(3); got != want {
		t.Errorf("version after Save = %d, want %d", got, want)
	}
	if saved.Dirty() {
		t.Error("a file that was saved must not be dirty")
	}
	onDisk, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got, want := string(onDisk), "x = 2\n"; got != want {
		t.Errorf("content on disk = %q, want %q", got, want)
	}
	if saved.ModTime().IsZero() {
		t.Error("a saved snapshot must carry the modification time of the file")
	}
}

// TestStoreVersionIsMonotonic checks that the version of a file grows with
// every mutation.
func TestStoreVersionIsMonotonic(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "")

	store := NewStore()
	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	previous := int64(0)
	for i := 0; i < 5; i++ {
		snapshot, err := file.Write([]byte("x = 1\n"))
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
		if snapshot.Version() <= previous {
			t.Fatalf("version = %d after the write %d, want a greater version than %d", snapshot.Version(), i, previous)
		}
		previous = snapshot.Version()
	}
}

// TestStoreNotifiesSubscribers checks that the subscribers of a store receive
// the events of the open files.
func TestStoreNotifiesSubscribers(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	store := NewStore()
	subscription := store.Subscribe()
	defer subscription.Close()

	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := file.Write([]byte("x = 2\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := file.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	want := []EventKind{EventOpen, EventWrite, EventSave}
	for _, kind := range want {
		select {
		case event := <-subscription.Events():
			if event.Kind != kind {
				t.Errorf("event kind = %q, want %q", event.Kind, kind)
			}
			if event.Path != path {
				t.Errorf("event path = %q, want %q", event.Path, path)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("the event %q was not delivered", kind)
		}
	}
}

// TestStoreCloseRemovesTheFile checks that a closed file is no longer
// available.
func TestStoreCloseRemovesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	store := NewStore()
	file, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(path); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := file.Read(); err == nil {
		t.Error("Read() of a closed file must fail")
	}
	if got := store.Paths(); len(got) != 0 {
		t.Errorf("store paths = %v, want no path", got)
	}
}

// TestStoreOpenMissingFile checks the error of a file that does not exist.
func TestStoreOpenMissingFile(t *testing.T) {
	store := NewStore()
	if _, err := store.Open(filepath.Join(t.TempDir(), "missing.jl")); err == nil {
		t.Fatal("Open() of a missing file must fail")
	}
}

// TestStoreOpenTwiceKeepsTheContent checks that opening a file twice does not
// lose the in-memory content.
func TestStoreOpenTwiceKeepsTheContent(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "a.jl", "x = 1\n")

	store := NewStore()
	first, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := first.Write([]byte("x = 2\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	second, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	snapshot, err := second.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got, want := snapshot.String(), "x = 2\n"; got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}
