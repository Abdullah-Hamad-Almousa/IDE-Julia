// Package workspace implements the project model of the jide engine.
//
// The package opens a folder as a workspace, detects the Julia environment of
// the folder (the Project.toml and the Manifest.toml of its root) and scans
// the source tree of the project.
//
// The scan resolves the symbolic links of the tree: a link that points
// outside the project root is reported instead of being followed, and no real
// path is visited twice, so a link that points at an ancestor of the root
// cannot make the scan loop.
//
// The package is headless: it has no dependency on a user interface, a
// windowing toolkit or a graphics device.
package workspace
