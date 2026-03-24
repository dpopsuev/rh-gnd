package dsr

import (
	"fmt"

	"github.com/dpopsuev/origami/toolkit"
)

// DepEdge declares that source From must be resolved before source To.
type DepEdge struct {
	From string
	To   string
}

// DepGraph is a directed acyclic graph of source dependencies.
type DepGraph struct {
	edges []DepEdge
}

// NewDepGraph creates a dependency graph with the given edges.
func NewDepGraph(edges ...DepEdge) *DepGraph {
	return &DepGraph{edges: edges}
}

// AddEdge registers a dependency: from must resolve before to.
func (g *DepGraph) AddEdge(from, to string) {
	g.edges = append(g.edges, DepEdge{From: from, To: to})
}

// TopologicalSort returns source names in resolution order.
// Sources with no dependencies come first. Returns an error if a cycle is detected.
func (g *DepGraph) TopologicalSort(sourceNames []string) ([]string, error) {
	if g == nil || len(g.edges) == 0 {
		return sourceNames, nil
	}

	nameSet := make(map[string]bool, len(sourceNames))
	for _, n := range sourceNames {
		nameSet[n] = true
	}

	inDegree := make(map[string]int, len(sourceNames))
	adj := make(map[string][]string)
	for _, n := range sourceNames {
		inDegree[n] = 0
	}

	for _, e := range g.edges {
		if !nameSet[e.From] || !nameSet[e.To] {
			continue
		}
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
	}

	var queue []string
	for _, n := range sourceNames {
		if inDegree[n] == 0 {
			queue = append(queue, n)
		}
	}

	var sorted []string
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, node)

		for _, neighbor := range adj[node] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	if len(sorted) != len(sourceNames) {
		return nil, fmt.Errorf("cycle detected in dependency graph (resolved %d of %d)", len(sorted), len(sourceNames))
	}
	return sorted, nil
}

// OrderSources returns the given sources reordered by dependency resolution order.
func (g *DepGraph) OrderSources(sources []toolkit.Source) ([]toolkit.Source, error) {
	names := make([]string, len(sources))
	byName := make(map[string]toolkit.Source, len(sources))
	for i, s := range sources {
		names[i] = s.Name
		byName[s.Name] = s
	}

	ordered, err := g.TopologicalSort(names)
	if err != nil {
		return nil, err
	}

	result := make([]toolkit.Source, 0, len(ordered))
	for _, name := range ordered {
		result = append(result, byName[name])
	}
	return result, nil
}
