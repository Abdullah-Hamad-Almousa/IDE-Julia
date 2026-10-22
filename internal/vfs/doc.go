// Package vfs implements the virtual file system of the jide engine.
//
// The package owns three things:
//
//   - immutable, versioned snapshots of file content (see Snapshot);
//   - a document store that keeps the open files of a session and notifies
//     subscribers about changes (see Store);
//   - file-system watching with a pluggable backend, plus a debouncing
//     pipeline that coalesces bursts of events (see Watcher, Debouncer and
//     Pipeline).
//
// The package is headless: it has no dependency on a user interface, a
// windowing toolkit or a graphics device.
package vfs
