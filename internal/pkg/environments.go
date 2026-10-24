package pkg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jide-dev/jide/internal/workspace"
)

var skippedDirectories = map[string]struct{}{
	".git": {}, ".julia": {}, ".vscode": {}, "node_modules": {},
	"vendor": {}, "build": {}, "dist": {},
}

// Environment describes a Julia project environment found on disk.
type Environment struct {
	Root     string
	Project  *workspace.Project
	Manifest *workspace.Manifest
}

// ListEnvironments finds Julia projects below roots. Roots may be directories
// or Project.toml files. Traversal does not follow symbolic links and ignores
// common generated and dependency directories. Results are sorted by project
// path, independent of filesystem enumeration order.
func ListEnvironments(ctx context.Context, roots ...string) ([]Environment, error) {
	if ctx == nil {
		return nil, errors.New("pkg: context is nil")
	}
	projects := make(map[string]struct{})
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(root) == "" {
			return nil, errors.New("pkg: environment search root is empty")
		}
		absolute, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("pkg: resolve environment root %q: %w", root, err)
		}
		info, err := os.Stat(absolute)
		if err != nil {
			return nil, fmt.Errorf("pkg: stat environment root %q: %w", absolute, err)
		}
		if !info.IsDir() {
			if filepath.Base(absolute) != workspace.ProjectFileName {
				continue
			}
			projects[absolute] = struct{}{}
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return nil, fmt.Errorf("pkg: resolve environment root symlinks %q: %w", absolute, err)
		}
		absolute = resolvedRoot
		err = filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() && path != absolute {
				if _, skip := skippedDirectories[entry.Name()]; skip {
					return filepath.SkipDir
				}
			}
			if !entry.IsDir() && entry.Name() == workspace.ProjectFileName {
				projects[path] = struct{}{}
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("pkg: scan environments under %q: %w", absolute, err)
		}
	}

	paths := make([]string, 0, len(projects))
	for path := range projects {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	environments := make([]Environment, 0, len(paths))
	for _, projectPath := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		opened, err := workspace.Open(filepath.Dir(projectPath))
		if err != nil {
			return nil, fmt.Errorf("pkg: open environment %q: %w", projectPath, err)
		}
		project, err := opened.Project()
		if err != nil {
			return nil, fmt.Errorf("pkg: parse environment %q: %w", projectPath, err)
		}
		manifest, err := opened.Manifest()
		if err != nil {
			return nil, fmt.Errorf("pkg: parse environment manifest in %q: %w", opened.Root(), err)
		}
		environment := Environment{Root: opened.Root(), Project: project, Manifest: manifest}
		environments = append(environments, environment)
	}
	return environments, nil
}
