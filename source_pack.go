package dsr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/origami/schematics/toolkit"
	yaml "go.yaml.in/yaml/v3"
)

// SourcePack is a composable collection of harvester sources for a specific
// operator or domain. Packs can include other packs to share common sources
// (e.g. OCP platform docs) without duplication.
type SourcePack struct {
	Name        string           `json:"name" yaml:"name"`
	Domain      string           `json:"domain,omitempty" yaml:"domain,omitempty"`
	Description string           `json:"description,omitempty" yaml:"description,omitempty"`
	VersionKey  string           `json:"version_key,omitempty" yaml:"version_key,omitempty"`
	Includes    []string         `json:"includes,omitempty" yaml:"includes,omitempty"`
	Repos       []SourcePackRepo `json:"repos,omitempty" yaml:"repos,omitempty"`
	Docs        []string         `json:"docs,omitempty" yaml:"docs,omitempty"`
}

// SourcePackRepo is a repository entry within a source pack. Branch is resolved
// at runtime via BranchPattern + envelope attributes.
type SourcePackRepo struct {
	Org           string   `json:"org" yaml:"org"`
	Name          string   `json:"name" yaml:"name"`
	Purpose       string   `json:"purpose,omitempty" yaml:"purpose,omitempty"`
	BranchPattern string   `json:"branch_pattern,omitempty" yaml:"branch_pattern,omitempty"`
	Exclude       []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

// PackResolver maps a pack name to its file path. Used by LoadPack to
// resolve included packs.
type PackResolver func(name string) (string, error)

// LoadPack reads a source pack YAML file and recursively resolves includes.
// The resolver maps included pack names to file paths. Repos are deduplicated
// by (org, name) — last-wins. Cycle detection prevents infinite recursion.
func LoadPack(path string, resolver PackResolver) (*SourcePack, error) {
	return loadPackRecursive(path, resolver, nil)
}

func loadPackRecursive(path string, resolver PackResolver, seen map[string]bool) (*SourcePack, error) {
	if seen == nil {
		seen = make(map[string]bool)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, err)
	}
	if seen[absPath] {
		return nil, fmt.Errorf("cycle detected: %s already included", path)
	}
	seen[absPath] = true

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source pack %s: %w", path, err)
	}

	var pack SourcePack
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		err = json.Unmarshal(data, &pack)
	default:
		err = yaml.Unmarshal(data, &pack)
	}
	if err != nil {
		return nil, fmt.Errorf("parse source pack %s: %w", path, err)
	}

	for _, inc := range pack.Includes {
		if resolver == nil {
			return nil, fmt.Errorf("include %q in %s but no resolver provided", inc, path)
		}
		incPath, err := resolver(inc)
		if err != nil {
			return nil, fmt.Errorf("resolve include %q: %w", inc, err)
		}
		child, err := loadPackRecursive(incPath, resolver, seen)
		if err != nil {
			return nil, fmt.Errorf("load include %q: %w", inc, err)
		}
		pack.Repos = mergeRepos(child.Repos, pack.Repos)
		pack.Docs = mergeDocs(child.Docs, pack.Docs)
	}

	return &pack, nil
}

// mergeRepos combines two repo lists, deduplicating by (org, name).
// Later entries override earlier ones (last-wins).
func mergeRepos(base, overlay []SourcePackRepo) []SourcePackRepo {
	index := make(map[string]int)
	var merged []SourcePackRepo

	for _, r := range base {
		key := r.Org + "/" + r.Name
		index[key] = len(merged)
		merged = append(merged, r)
	}
	for _, r := range overlay {
		key := r.Org + "/" + r.Name
		if idx, ok := index[key]; ok {
			merged[idx] = r
		} else {
			index[key] = len(merged)
			merged = append(merged, r)
		}
	}
	return merged
}

func mergeDocs(base, overlay []string) []string {
	seen := make(map[string]bool, len(base)+len(overlay))
	var merged []string
	for _, d := range base {
		if !seen[d] {
			seen[d] = true
			merged = append(merged, d)
		}
	}
	for _, d := range overlay {
		if !seen[d] {
			seen[d] = true
			merged = append(merged, d)
		}
	}
	return merged
}

// ToSources converts a SourcePack into a flat list of Source entries.
// Branches are resolved using the provided attributes map and each repo's
// BranchPattern. If a repo has no BranchPattern, the pack-level VersionKey
// is substituted as "{version_key}" so ResolveBranch can still interpolate.
func (p *SourcePack) ToSources(attrs map[string]string) []toolkit.Source {
	var sources []toolkit.Source
	for _, r := range p.Repos {
		pattern := r.BranchPattern
		if pattern == "" && p.VersionKey != "" {
			pattern = "{" + p.VersionKey + "}"
		}
		branch := ResolveBranch(pattern, attrs)

		s := toolkit.Source{
			Name:          r.Name,
			Kind:          toolkit.SourceKindRepo,
			URI:           fmt.Sprintf("https://github.com/%s/%s", r.Org, r.Name),
			Purpose:       r.Purpose,
			Branch:        branch,
			Org:           r.Org,
			BranchPattern: r.BranchPattern,
			Exclude:       r.Exclude,
			Tags: map[string]string{
				"layer": "base",
			},
		}
		if p.Domain != "" {
			s.Tags["domain"] = p.Domain
		}
		sources = append(sources, s)
	}
	for _, d := range p.Docs {
		sources = append(sources, toolkit.Source{
			Name:    filepath.Base(d),
			Kind:    toolkit.SourceKindDoc,
			URI:     d,
			Purpose: "Documentation",
		})
	}
	return sources
}

// MergePacks combines multiple source packs, deduplicating repos by (org, name).
func MergePacks(packs ...*SourcePack) *SourcePack {
	if len(packs) == 0 {
		return &SourcePack{}
	}
	merged := &SourcePack{
		Name: packs[0].Name,
	}
	for _, p := range packs {
		merged.Repos = mergeRepos(merged.Repos, p.Repos)
		merged.Docs = mergeDocs(merged.Docs, p.Docs)
	}
	return merged
}
