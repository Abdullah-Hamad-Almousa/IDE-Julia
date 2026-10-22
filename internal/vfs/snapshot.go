package vfs

import "time"

// Snapshot is an immutable, versioned view of the content of a file.
//
// A snapshot never changes. Every mutation of a file produces a new snapshot
// whose version is strictly greater than the version of the previous one, so
// a consumer can order the revisions of a document without comparing
// timestamps.
type Snapshot struct {
	path    string
	version int64
	content []byte
	modTime time.Time
	size    int64
	dirty   bool
}

// NewSnapshot returns a snapshot of the given content.
//
// The content is copied, so the caller may reuse the slice it passes in.
func NewSnapshot(path string, version int64, content []byte, modTime time.Time) Snapshot {
	buffer := make([]byte, len(content))
	copy(buffer, content)
	return Snapshot{
		path:    path,
		version: version,
		content: buffer,
		modTime: modTime,
		size:    int64(len(buffer)),
	}
}

// Path returns the absolute path of the file.
func (s Snapshot) Path() string { return s.path }

// Version returns the revision of the snapshot.
//
// The version starts at 1 for a file that was read from disk and grows
// strictly monotonically with every write and every save.
func (s Snapshot) Version() int64 { return s.version }

// ModTime returns the modification time of the file.
func (s Snapshot) ModTime() time.Time { return s.modTime }

// Size returns the size of the content in bytes.
func (s Snapshot) Size() int64 { return s.size }

// Dirty reports whether the content of the snapshot differs from the file on
// disk.
func (s Snapshot) Dirty() bool { return s.dirty }

// Bytes returns a copy of the content of the snapshot.
func (s Snapshot) Bytes() []byte {
	buffer := make([]byte, len(s.content))
	copy(buffer, s.content)
	return buffer
}

// String returns the content of the snapshot as a string.
func (s Snapshot) String() string { return string(s.content) }
