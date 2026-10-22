package workspace

import (
	"slices"
)

// Manifest is the parsed content of a Julia Manifest.toml file.
//
// The type covers the two manifest formats of Julia: format 2.0 stores the
// packages in the [deps] table, format 1.0 stores them in the top level
// tables of the document.
type Manifest struct {
	// Path is the absolute path of the Manifest.toml file.
	Path string
	// Format is the manifest format version, for example "2.0".
	Format string
	// JuliaVersion is the Julia version the manifest was resolved with.
	JuliaVersion string
	// ProjectHash identifies the project the manifest belongs to.
	ProjectHash string
	// Packages holds the resolved packages, sorted by name.
	Packages []ManifestPackage
}

// ManifestPackage is one resolved package of a manifest.
type ManifestPackage struct {
	// Name is the package name.
	Name string
	// UUID is the package UUID.
	UUID string
	// Version is the resolved version.
	Version string
	// TreeHash is the source tree hash of the package. It is empty for the
	// manifest format 2.0, which does not record it.
	TreeHash string
	// Deps holds the names of the direct dependencies of the package.
	Deps []string
}

// Package returns the resolved package with the given name.
//
// The second result reports whether the manifest holds such a package.
func (m *Manifest) Package(name string) (ManifestPackage, bool) {
	for _, resolved := range m.Packages {
		if resolved.Name == name {
			return resolved, true
		}
	}
	return ManifestPackage{}, false
}

// ParseManifest reads and parses the Manifest.toml file at path.
//
// The function fails when the file cannot be read or when it is not valid
// TOML.
func ParseManifest(path string) (*Manifest, error) {
	document, err := parseTOMLFile(path)
	if err != nil {
		return nil, err
	}

	manifest := &Manifest{Path: path}
	manifest.Format, _ = stringValue(document["manifest_format"])
	manifest.JuliaVersion, _ = stringValue(document["julia_version"])
	manifest.ProjectHash, _ = stringValue(document["project_hash"])

	packages := make(map[string]ManifestPackage)

	// The manifest format 2.0 stores the packages in the [deps] table.
	if deps, ok := document["deps"].(map[string]any); ok {
		for name, entry := range deps {
			if resolved, ok := packageFromTable(name, entry); ok {
				packages[name] = resolved
			}
		}
	}

	// The manifest format 1.0 stores the packages in the top level tables
	// of the document.
	for name, entry := range document {
		if isManifestKey(name) {
			continue
		}
		if resolved, ok := packageFromTable(name, entry); ok {
			packages[name] = resolved
		}
	}

	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	slices.Sort(names)

	manifest.Packages = make([]ManifestPackage, 0, len(names))
	for _, name := range names {
		manifest.Packages = append(manifest.Packages, packages[name])
	}
	return manifest, nil
}

// isManifestKey reports whether the key holds a scalar of a manifest instead
// of a package.
func isManifestKey(name string) bool {
	switch name {
	case "deps", "manifest_format", "julia_version", "project_hash":
		return true
	default:
		return false
	}
}

// packageFromTable builds a package from the table of a manifest.
//
// A package is stored as a table or as an array of tables, depending on the
// manifest format.
func packageFromTable(name string, value any) (ManifestPackage, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return packageFromFields(name, typed)
	case []any:
		if len(typed) == 0 {
			return ManifestPackage{}, false
		}
		table, ok := typed[0].(map[string]any)
		if !ok {
			return ManifestPackage{}, false
		}
		return packageFromFields(name, table)
	case []map[string]any:
		if len(typed) == 0 {
			return ManifestPackage{}, false
		}
		return packageFromFields(name, typed[0])
	default:
		return ManifestPackage{}, false
	}
}

// packageFromFields builds a package from the fields of its table.
func packageFromFields(name string, table map[string]any) (ManifestPackage, bool) {
	uuid, ok := stringValue(table["uuid"])
	if !ok {
		return ManifestPackage{}, false
	}
	resolved := ManifestPackage{Name: name, UUID: uuid}
	resolved.Version, _ = stringValue(table["version"])
	resolved.TreeHash, _ = stringValue(table["git-tree-sha1"])
	if hash, ok := stringValue(table["tree_hash"]); ok && resolved.TreeHash == "" {
		resolved.TreeHash = hash
	}
	resolved.Deps = stringSlice(table["deps"])
	return resolved, true
}
