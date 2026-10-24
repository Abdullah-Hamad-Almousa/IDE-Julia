package pkg

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jide-dev/jide/internal/workspace"
)

const (
	addScript       = `using Pkg; Pkg.activate(ARGS[1]); Pkg.add(ARGS[2])`
	removeScript    = `using Pkg; Pkg.activate(ARGS[1]); Pkg.rm(ARGS[2])`
	updateAllScript = `using Pkg; Pkg.activate(ARGS[1]); Pkg.update()`
	updateOneScript = `using Pkg; Pkg.activate(ARGS[1]); Pkg.update([Pkg.PackageSpec(name=ARGS[2])])`
)

// Add installs package into the project environment and returns Pkg output.
func (m *Manager) Add(ctx context.Context, projectPath, packageName string) (string, error) {
	if err := validatePackageName(packageName); err != nil {
		return "", err
	}
	output, err := m.runPkg(ctx, projectPath, addScript, packageName)
	return string(output), err
}

// Remove removes package from the project environment and returns Pkg output.
func (m *Manager) Remove(ctx context.Context, projectPath, packageName string) (string, error) {
	if err := validatePackageName(packageName); err != nil {
		return "", err
	}
	output, err := m.runPkg(ctx, projectPath, removeScript, packageName)
	return string(output), err
}

// Upgrade updates all dependencies when packageName is empty, or only the
// named dependency otherwise. It returns Pkg output.
func (m *Manager) Upgrade(ctx context.Context, projectPath, packageName string) (string, error) {
	if packageName == "" {
		output, err := m.runPkg(ctx, projectPath, updateAllScript)
		return string(output), err
	}
	if err := validatePackageName(packageName); err != nil {
		return "", err
	}
	output, err := m.runPkg(ctx, projectPath, updateOneScript, packageName)
	return string(output), err
}

// ManifestDiff compares two manifests and reports added, removed and changed
// packages in stable name/UUID order.
type ManifestDiff struct {
	Added   []workspace.ManifestPackage
	Removed []workspace.ManifestPackage
	Updated []PackageUpdate
}

// PackageUpdate describes a package whose resolved data changed.
type PackageUpdate struct {
	Before workspace.ManifestPackage
	After  workspace.ManifestPackage
}

// DiffManifests compares before and after manifests by UUID, using name as a
// fallback for entries without UUIDs.
func DiffManifests(before, after *workspace.Manifest) ManifestDiff {
	oldPackages := packageMap(before)
	newPackages := packageMap(after)
	diff := ManifestDiff{
		Added:   []workspace.ManifestPackage{},
		Removed: []workspace.ManifestPackage{},
		Updated: []PackageUpdate{},
	}
	for key, oldPackage := range oldPackages {
		newPackage, found := newPackages[key]
		if !found {
			diff.Removed = append(diff.Removed, oldPackage)
			continue
		}
		if !samePackage(oldPackage, newPackage) {
			diff.Updated = append(diff.Updated, PackageUpdate{Before: oldPackage, After: newPackage})
		}
	}
	for key, newPackage := range newPackages {
		if _, found := oldPackages[key]; !found {
			diff.Added = append(diff.Added, newPackage)
		}
	}
	sortPackages(diff.Added)
	sortPackages(diff.Removed)
	// packageMap keys are UUIDs, so sort updates explicitly for stable output.
	slices.SortFunc(diff.Updated, func(a, b PackageUpdate) int {
		if packageLess(a.After, b.After) {
			return -1
		}
		if packageLess(b.After, a.After) {
			return 1
		}
		return 0
	})
	return diff
}

// DiffManifestFiles parses and compares two Manifest.toml files.
func DiffManifestFiles(beforePath, afterPath string) (ManifestDiff, error) {
	before, err := workspace.ParseManifest(beforePath)
	if err != nil {
		return ManifestDiff{}, fmt.Errorf("pkg: parse previous manifest: %w", err)
	}
	after, err := workspace.ParseManifest(afterPath)
	if err != nil {
		return ManifestDiff{}, fmt.Errorf("pkg: parse current manifest: %w", err)
	}
	return DiffManifests(before, after), nil
}

func validatePackageName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("pkg: package name is empty")
	}
	return nil
}

func packageMap(manifest *workspace.Manifest) map[string]workspace.ManifestPackage {
	packages := make(map[string]workspace.ManifestPackage)
	if manifest == nil {
		return packages
	}
	for _, resolved := range manifest.Packages {
		key := resolved.UUID
		if key == "" {
			key = "name:" + resolved.Name
		}
		packages[key] = resolved
	}
	return packages
}

func samePackage(a, b workspace.ManifestPackage) bool {
	if a.Name != b.Name || a.UUID != b.UUID || a.Version != b.Version || a.TreeHash != b.TreeHash {
		return false
	}
	aDeps := append([]string(nil), a.Deps...)
	bDeps := append([]string(nil), b.Deps...)
	slices.Sort(aDeps)
	slices.Sort(bDeps)
	return slices.Equal(aDeps, bDeps)
}

func sortPackages(packages []workspace.ManifestPackage) {
	slices.SortFunc(packages, func(a, b workspace.ManifestPackage) int {
		if packageLess(a, b) {
			return -1
		}
		if packageLess(b, a) {
			return 1
		}
		return 0
	})
}

func packageLess(a, b workspace.ManifestPackage) bool {
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.UUID < b.UUID
}
