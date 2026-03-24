package dsr

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/origami/toolkit"
)

// OfflineFSDriver serves GND sources from a pre-staged offline bundle
// stored in an fs.FS. It handles both repo and doc source kinds by reading
// files directly from the bundle filesystem.
//
// The bundle is expected to have the structure:
//
//	repos/<name>/...   — source files for each repo
//	docs/<path>        — documentation files
type OfflineFSDriver struct {
	fsys fs.FS
	kind toolkit.SourceKind
}

// NewOfflineFSDriver creates a driver that reads from the given filesystem.
// The kind parameter determines which source kind this driver handles
// (typically SourceKindRepo or SourceKindDoc).
func NewOfflineFSDriver(fsys fs.FS, kind toolkit.SourceKind) *OfflineFSDriver {
	return &OfflineFSDriver{fsys: fsys, kind: kind}
}

func (d *OfflineFSDriver) Handles() toolkit.SourceKind {
	return d.kind
}

func (d *OfflineFSDriver) Ensure(_ context.Context, _ toolkit.Source) error {
	return nil
}

func (d *OfflineFSDriver) Search(_ context.Context, src toolkit.Source, query string, maxResults int) ([]toolkit.SearchResult, error) {
	root := d.sourceRoot(src)
	var results []toolkit.SearchResult
	queryLower := strings.ToLower(query)

	fs.WalkDir(d.fsys, root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || len(results) >= maxResults {
			return nil
		}
		data, readErr := fs.ReadFile(d.fsys, path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		if !strings.Contains(strings.ToLower(content), queryLower) {
			return nil
		}
		relPath, _ := filepath.Rel(root, path)
		for i, line := range strings.Split(content, "\n") {
			if strings.Contains(strings.ToLower(line), queryLower) {
				results = append(results, toolkit.SearchResult{
					Source:  src.Name,
					Path:    relPath,
					Line:    i + 1,
					Snippet: strings.TrimSpace(line),
				})
				break
			}
		}
		return nil
	})
	return results, nil
}

func (d *OfflineFSDriver) Read(_ context.Context, src toolkit.Source, path string) ([]byte, error) {
	root := d.sourceRoot(src)
	fullPath := filepath.Join(root, path)
	data, err := fs.ReadFile(d.fsys, fullPath)
	if err != nil {
		return nil, fmt.Errorf("offline read %s/%s: %w", src.Name, path, err)
	}
	return data, nil
}

func (d *OfflineFSDriver) List(_ context.Context, src toolkit.Source, root string, maxDepth int) ([]toolkit.ContentEntry, error) {
	srcRoot := d.sourceRoot(src)
	searchRoot := filepath.Join(srcRoot, root)
	var entries []toolkit.ContentEntry

	fs.WalkDir(d.fsys, searchRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(searchRoot, path)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator))
		if maxDepth > 0 && depth >= maxDepth {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, _ := entry.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}
		entries = append(entries, toolkit.ContentEntry{
			Path:  rel,
			IsDir: entry.IsDir(),
			Size:  size,
		})
		return nil
	})
	return entries, nil
}

func (d *OfflineFSDriver) sourceRoot(src toolkit.Source) string {
	switch d.kind {
	case toolkit.SourceKindRepo:
		return "repos/" + src.Name
	case toolkit.SourceKindDoc:
		if src.LocalPath != "" {
			return filepath.Dir(src.LocalPath)
		}
		return "docs/" + src.Name
	default:
		return src.Name
	}
}

var _ toolkit.Driver = (*OfflineFSDriver)(nil)
