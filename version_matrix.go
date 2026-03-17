package dsr

import (
	"strings"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

// VersionEntry maps a version string to the concrete branch, tag, and
// documentation URL patterns for that version.
type VersionEntry struct {
	Version string `json:"version" yaml:"version"`
	Branch  string `json:"branch" yaml:"branch"`
	Tag     string `json:"tag,omitempty" yaml:"tag,omitempty"`
	DocsURL string `json:"docs_url,omitempty" yaml:"docs_url,omitempty"`
}

// VersionMatrix resolves version tags to branch names, Git tags, and
// documentation URLs.
type VersionMatrix struct {
	Entries []VersionEntry `json:"entries" yaml:"entries"`
}

// NewVersionMatrix creates an empty matrix.
func NewVersionMatrix(entries ...VersionEntry) *VersionMatrix {
	return &VersionMatrix{Entries: entries}
}

// Resolve finds the best VersionEntry for the given version string.
// Exact match takes priority; then longest prefix match.
func (vm *VersionMatrix) Resolve(version string) *VersionEntry {
	if vm == nil || len(vm.Entries) == 0 {
		return nil
	}

	for i := range vm.Entries {
		if vm.Entries[i].Version == version {
			return &vm.Entries[i]
		}
	}

	var best *VersionEntry
	bestLen := 0
	for i := range vm.Entries {
		if strings.HasPrefix(version, vm.Entries[i].Version) && len(vm.Entries[i].Version) > bestLen {
			best = &vm.Entries[i]
			bestLen = len(vm.Entries[i].Version)
		}
	}
	return best
}

// ResolveBranch returns the branch name for a version, or fallback if not found.
func (vm *VersionMatrix) ResolveBranch(version, fallback string) string {
	entry := vm.Resolve(version)
	if entry == nil || entry.Branch == "" {
		return fallback
	}
	return entry.Branch
}

// ResolveSource returns a copy of the source with its Branch set to the
// version-resolved branch. If the version is not in the matrix, the
// source is returned unchanged.
func (vm *VersionMatrix) ResolveSource(src toolkit.Source, version string) toolkit.Source {
	entry := vm.Resolve(version)
	if entry == nil {
		return src
	}
	out := src
	if entry.Branch != "" {
		out.Branch = entry.Branch
	}
	return out
}
