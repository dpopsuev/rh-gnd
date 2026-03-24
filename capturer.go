package dsr

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dpopsuev/origami/calibrate"
	"github.com/dpopsuev/origami/toolkit"
)

const gndSchematic = "gnd"

// Capturer captures a GND bundle from live sources using a
// SourceReader. It writes repos/ and docs/ to disk and records a manifest.
type Capturer struct {
	reader toolkit.SourceReader
	logger *slog.Logger
}

var _ calibrate.Capturer = (*Capturer)(nil)

// NewCapturer creates a capturer that uses the given reader
// (typically an AccessRouter configured with GitDriver + DocsDriver).
func NewCapturer(reader toolkit.SourceReader, logger *slog.Logger) *Capturer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Capturer{reader: reader, logger: logger}
}

func (c *Capturer) Schematic() string { return gndSchematic }

// Capture reads a source pack, fetches all content via the SourceReader,
// writes it to cfg.OutputDir in the offline bundle layout, and produces
// manifest.yaml.
func (c *Capturer) Capture(ctx context.Context, cfg calibrate.CaptureConfig) error {
	pack, err := LoadPack(cfg.SourcePack, nil)
	if err != nil {
		return fmt.Errorf("load source pack: %w", err)
	}

	outDir := cfg.OutputDir
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	sources := pack.ToSources(nil)

	var manifest calibrate.Manifest
	manifest.SchemaVersion = calibrate.SchemaV1
	manifest.Schematic = gndSchematic
	manifest.CapturedAt = time.Now().UTC()

	for _, src := range sources {
		if err := ctx.Err(); err != nil {
			return err
		}

		switch src.Kind {
		case toolkit.SourceKindRepo:
			entry, err := c.captureRepo(ctx, src, outDir)
			if err != nil {
				c.logger.Warn("skipping repo", "name", src.Name, "error", err)
				continue
			}
			manifest.Repos = append(manifest.Repos, entry)

		case toolkit.SourceKindDoc:
			entry, err := c.captureDoc(ctx, src, outDir)
			if err != nil {
				c.logger.Warn("skipping doc", "name", src.Name, "error", err)
				continue
			}
			manifest.Docs = append(manifest.Docs, entry)
		}
	}

	return calibrate.WriteManifest(outDir, &manifest)
}

func (c *Capturer) captureRepo(ctx context.Context, src toolkit.Source, outDir string) (calibrate.RepoEntry, error) {
	c.logger.Info("capturing repo", "name", src.Name, "branch", src.Branch)

	if err := c.reader.Ensure(ctx, src); err != nil {
		return calibrate.RepoEntry{}, fmt.Errorf("ensure %s: %w", src.Name, err)
	}

	entries, err := c.reader.List(ctx, src, ".", 0)
	if err != nil {
		return calibrate.RepoEntry{}, fmt.Errorf("list %s: %w", src.Name, err)
	}

	repoDir := filepath.Join(outDir, "repos", src.Name)
	hasher := sha256.New()
	var files []string

	for _, e := range entries {
		if e.IsDir {
			continue
		}
		if isExcluded(e.Path, src.Exclude) {
			continue
		}

		data, err := c.reader.Read(ctx, src, e.Path)
		if err != nil {
			c.logger.Debug("skipping file", "repo", src.Name, "path", e.Path, "error", err)
			continue
		}

		destPath := filepath.Join(repoDir, e.Path)
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return calibrate.RepoEntry{}, err
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return calibrate.RepoEntry{}, err
		}

		hasher.Write([]byte(e.Path))
		hasher.Write(data)
		files = append(files, e.Path)
	}

	sort.Strings(files)

	return calibrate.RepoEntry{
		Name:   src.Name,
		Branch: src.Branch,
		SHA:    fmt.Sprintf("%x", hasher.Sum(nil)),
		Files:  files,
	}, nil
}

func (c *Capturer) captureDoc(ctx context.Context, src toolkit.Source, outDir string) (calibrate.DocEntry, error) {
	c.logger.Info("capturing doc", "name", src.Name)

	data, err := c.reader.Read(ctx, src, "/")
	if err != nil {
		return calibrate.DocEntry{}, fmt.Errorf("read doc %s: %w", src.Name, err)
	}

	localPath := filepath.Join("docs", src.Name+".md")
	destPath := filepath.Join(outDir, localPath)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return calibrate.DocEntry{}, err
	}
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return calibrate.DocEntry{}, err
	}

	h := sha256.Sum256(data)
	return calibrate.DocEntry{
		Name:      src.Name,
		LocalPath: localPath,
		SHA:       fmt.Sprintf("%x", h),
	}, nil
}

// isExcluded checks if a path matches any exclusion pattern.
func isExcluded(path string, exclude []string) bool {
	for _, pattern := range exclude {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}
	return false
}

// Validator validates a GND bundle's structure.
type Validator struct{}

var _ calibrate.BundleValidator = (*Validator)(nil)

func (v *Validator) Schematic() string { return gndSchematic }

// Validate checks the bundle layout and manifest integrity.
func (v *Validator) Validate(fsys fs.FS) []error {
	errs := calibrate.ValidateBundle(fsys, false)

	if _, err := fs.Stat(fsys, "repos"); err != nil {
		errs = append(errs, fmt.Errorf("missing repos/ directory"))
	}

	m, err := calibrate.ReadManifest(fsys)
	if err != nil {
		return errs
	}

	if m.Schematic != gndSchematic {
		errs = append(errs, fmt.Errorf("manifest schematic is %q, expected %q", m.Schematic, gndSchematic))
	}
	if m.SchemaVersion != calibrate.SchemaV1 {
		errs = append(errs, fmt.Errorf("unsupported schema version %q", m.SchemaVersion))
	}

	return errs
}
