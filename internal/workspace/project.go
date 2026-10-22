package workspace

import (
	"fmt"
	"os"
	"slices"
	"strconv"

	"github.com/pelletier/go-toml/v2"
)

// Project is the parsed content of a Julia Project.toml file.
//
// The type is immutable: the resolved versions of the dependencies are filled
// in when the project is parsed together with its manifest.
type Project struct {
	// Path is the absolute path of the Project.toml file.
	Path string
	// Name is the package name of the project.
	Name string
	// UUID is the package UUID of the project.
	UUID string
	// Version is the package version of the project.
	Version string
	// Compat holds the [compat] section of the project.
	Compat map[string]string
	// Deps holds the direct dependencies of the project.
	Deps []Dependency
	// WeakDeps holds the weak dependencies of the project.
	WeakDeps []Dependency
	// Extras holds the extra packages of the project.
	Extras []Dependency
	// Targets holds the [targets] section of the project.
	Targets map[string][]string
}

// Dependency is one dependency of a Julia project.
type Dependency struct {
	// Name is the package name.
	Name string
	// UUID is the package UUID.
	UUID string
	// Version is the resolved version. It is empty when the version is
	// unknown, for example when the project has no manifest.
	Version string
	// Weak reports whether the dependency is a weak dependency.
	Weak bool
	// Deps holds the names of the direct dependencies of the package. It is
	// empty when the package is not resolved.
	Deps []string
}

// JuliaCompat returns the Julia version compatibility of the project.
//
// The value is empty when the project does not constrain the Julia version.
func (p *Project) JuliaCompat() string { return p.Compat["julia"] }

// ParseProject reads and parses the Project.toml file at path.
//
// The function fails when the file cannot be read or when it is not valid
// TOML.
func ParseProject(path string) (*Project, error) {
	document, err := parseTOMLFile(path)
	if err != nil {
		return nil, err
	}

	project := &Project{
		Path:     path,
		Compat:   stringMap(document["compat"]),
		Deps:     dependencyList(document["deps"], false),
		WeakDeps: dependencyList(document["weakdeps"], true),
		Extras:   dependencyList(document["extras"], false),
		Targets:  stringSliceMap(document["targets"]),
	}
	project.Name, _ = stringValue(document["name"])
	project.UUID, _ = stringValue(document["uuid"])
	project.Version, _ = stringValue(document["version"])
	return project, nil
}

// resolve fills the resolved versions and the dependencies of the project
// from the manifest.
func (p *Project) resolve(manifest *Manifest) {
	for i := range p.Deps {
		p.Deps[i].resolve(manifest)
	}
	for i := range p.WeakDeps {
		p.WeakDeps[i].resolve(manifest)
	}
	for i := range p.Extras {
		p.Extras[i].resolve(manifest)
	}
}

// resolve fills the resolved version and the dependencies of the dependency
// from the manifest.
func (d *Dependency) resolve(manifest *Manifest) {
	resolved, ok := manifest.Package(d.Name)
	if !ok {
		return
	}
	d.Version = resolved.Version
	d.Deps = resolved.Deps
}

// parseTOMLFile reads a file and parses it as a TOML document.
func parseTOMLFile(path string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("workspace: read %q: %w", path, err)
	}
	var document map[string]any
	if err := toml.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("workspace: parse %q: %w", path, err)
	}
	return document, nil
}

// stringValue returns the textual form of a TOML scalar.
func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(typed), true
	default:
		return "", false
	}
}

// stringMap returns the textual values of a TOML table.
//
// The result is nil when the value is not a table.
func stringMap(value any) map[string]string {
	table, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(table))
	for key, entry := range table {
		if text, ok := stringValue(entry); ok {
			result[key] = text
		}
	}
	return result
}

// stringSlice returns the textual values of a TOML array.
func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := stringValue(item); ok {
			result = append(result, text)
		}
	}
	return result
}

// stringSliceMap returns the arrays of a TOML table.
func stringSliceMap(value any) map[string][]string {
	table, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string][]string, len(table))
	for key, entry := range table {
		values := stringSlice(entry)
		if values == nil {
			continue
		}
		result[key] = values
	}
	return result
}

// dependencyList returns the dependencies of a TOML table of UUIDs.
//
// The dependencies are sorted by name, so the result does not depend on the
// order of the table.
func dependencyList(value any, weak bool) []Dependency {
	table, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(table))
	for name := range table {
		names = append(names, name)
	}
	slices.Sort(names)

	dependencies := make([]Dependency, 0, len(names))
	for _, name := range names {
		uuid, _ := stringValue(table[name])
		dependencies = append(dependencies, Dependency{Name: name, UUID: uuid, Weak: weak})
	}
	return dependencies
}
