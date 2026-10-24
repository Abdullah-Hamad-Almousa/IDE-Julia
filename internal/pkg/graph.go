package pkg

import (
	"slices"

	"github.com/jide-dev/jide/internal/workspace"
)

// DependencyGraph is a deterministic graph of the resolved packages in a
// Julia manifest. DirectDependencies identifies the project-level roots.
type DependencyGraph struct {
	DirectDependencies []string
	Nodes              []DependencyNode
}

// DependencyNode is one package and its direct dependency names.
type DependencyNode struct {
	Name         string
	UUID         string
	Version      string
	Dependencies []string
}

// BuildDependencyGraph builds a dependency graph from a project and manifest.
// The project may be nil; when present, only declared project dependencies
// are included as direct roots. Manifest dependencies not present in the
// manifest remain listed on their parent node, allowing callers to flag
// incomplete or partially resolved environments.
func BuildDependencyGraph(project *workspace.Project, manifest *workspace.Manifest) DependencyGraph {
	graph := DependencyGraph{DirectDependencies: []string{}, Nodes: []DependencyNode{}}
	if project != nil {
		for _, dependency := range project.Deps {
			graph.DirectDependencies = append(graph.DirectDependencies, dependency.Name)
		}
		slices.Sort(graph.DirectDependencies)
	}
	if manifest == nil {
		return graph
	}
	for _, resolved := range manifest.Packages {
		dependencies := append([]string(nil), resolved.Deps...)
		slices.Sort(dependencies)
		graph.Nodes = append(graph.Nodes, DependencyNode{
			Name: resolved.Name, UUID: resolved.UUID, Version: resolved.Version,
			Dependencies: dependencies,
		})
	}
	slices.SortFunc(graph.Nodes, func(a, b DependencyNode) int {
		if a.Name != b.Name {
			return compareString(a.Name, b.Name)
		}
		return compareString(a.UUID, b.UUID)
	})
	return graph
}

func compareString(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}
