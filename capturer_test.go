package dsr_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/origami/calibrate"
	"github.com/dpopsuev/rh-gnd"
	"github.com/dpopsuev/origami/schematics/toolkit"
)

type capturerStubDriver struct {
	kind  toolkit.SourceKind
	files map[string]map[string][]byte // source name -> path -> content
}

func (d *capturerStubDriver) Handles() toolkit.SourceKind { return d.kind }

func (d *capturerStubDriver) Ensure(_ context.Context, _ toolkit.Source) error { return nil }

func (d *capturerStubDriver) Search(_ context.Context, _ toolkit.Source, _ string, _ int) ([]toolkit.SearchResult, error) {
	return nil, nil
}

func (d *capturerStubDriver) Read(_ context.Context, src toolkit.Source, path string) ([]byte, error) {
	if files, ok := d.files[src.Name]; ok {
		if data, ok := files[path]; ok {
			return data, nil
		}
	}
	if d.kind == toolkit.SourceKindDoc {
		if files, ok := d.files[src.Name]; ok {
			if data, ok := files["/"]; ok {
				return data, nil
			}
		}
	}
	return nil, os.ErrNotExist
}

func (d *capturerStubDriver) List(_ context.Context, src toolkit.Source, _ string, _ int) ([]toolkit.ContentEntry, error) {
	files, ok := d.files[src.Name]
	if !ok {
		return nil, nil
	}
	var entries []toolkit.ContentEntry
	for path := range files {
		if path == "/" {
			continue
		}
		entries = append(entries, toolkit.ContentEntry{Path: path})
	}
	return entries, nil
}

func TestCapturer_CaptureAndValidate(t *testing.T) {
	repoDriver := &capturerStubDriver{
		kind: toolkit.SourceKindRepo,
		files: map[string]map[string][]byte{
			"test-repo": {
				"main.go":      []byte("package main"),
				"pkg/util.go":  []byte("package util"),
			},
		},
	}
	docDriver := &capturerStubDriver{
		kind: toolkit.SourceKindDoc,
		files: map[string]map[string][]byte{
			"test-doc": {
				"/": []byte("# Test Documentation"),
			},
		},
	}

	router := dsr.NewRouter(
		dsr.WithGitDriver(repoDriver),
		dsr.WithDocsDriver(docDriver),
	)

	packDir := t.TempDir()
	packPath := filepath.Join(packDir, "source_pack.yaml")
	packContent := `
name: test-pack
repos:
  - org: test-org
    name: test-repo
docs:
  - test-doc
`
	if err := os.WriteFile(packPath, []byte(packContent), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	capturer := dsr.NewCapturer(router, nil)

	if capturer.Schematic() != "gnd" {
		t.Fatalf("Schematic() = %q, want %q", capturer.Schematic(), "gnd")
	}

	cfg := calibrate.CaptureConfig{
		Schematic:  "gnd",
		SourcePack: packPath,
		OutputDir:  outDir,
	}

	if err := capturer.Capture(context.Background(), cfg); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// Verify manifest exists and is readable.
	m, err := calibrate.ReadManifest(os.DirFS(outDir))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}

	if m.SchemaVersion != calibrate.SchemaV1 {
		t.Errorf("SchemaVersion = %q, want %q", m.SchemaVersion, calibrate.SchemaV1)
	}
	if m.Schematic != "gnd" {
		t.Errorf("Schematic = %q, want %q", m.Schematic, "gnd")
	}
	if m.CapturedAt.IsZero() {
		t.Error("CapturedAt is zero")
	}

	if len(m.Repos) != 1 {
		t.Fatalf("Repos count = %d, want 1", len(m.Repos))
	}
	if m.Repos[0].Name != "test-repo" {
		t.Errorf("Repo.Name = %q, want %q", m.Repos[0].Name, "test-repo")
	}
	if len(m.Repos[0].Files) != 2 {
		t.Errorf("Repo.Files count = %d, want 2", len(m.Repos[0].Files))
	}
	if m.Repos[0].SHA == "" {
		t.Error("Repo.SHA is empty")
	}

	if len(m.Docs) != 1 {
		t.Fatalf("Docs count = %d, want 1", len(m.Docs))
	}
	if m.Docs[0].SHA == "" {
		t.Error("Doc.SHA is empty")
	}

	// Verify files on disk.
	mainGo, err := os.ReadFile(filepath.Join(outDir, "repos", "test-repo", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if string(mainGo) != "package main" {
		t.Errorf("main.go content = %q", string(mainGo))
	}

	docFile, err := os.ReadFile(filepath.Join(outDir, "docs", "test-doc.md"))
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}
	if string(docFile) != "# Test Documentation" {
		t.Errorf("doc content = %q", string(docFile))
	}

	// Validate bundle.
	errs := calibrate.ValidateBundle(os.DirFS(outDir), false)
	if len(errs) != 0 {
		t.Errorf("ValidateBundle errors: %v", errs)
	}

	// GND-specific validation.
	validator := &dsr.Validator{}
	if validator.Schematic() != "gnd" {
		t.Errorf("validator Schematic() = %q", validator.Schematic())
	}
	vErrs := validator.Validate(os.DirFS(outDir))
	if len(vErrs) != 0 {
		t.Errorf("Validator errors: %v", vErrs)
	}
}

func TestCapturer_Idempotent(t *testing.T) {
	repoDriver := &capturerStubDriver{
		kind: toolkit.SourceKindRepo,
		files: map[string]map[string][]byte{
			"stable-repo": {
				"file.go": []byte("package stable"),
			},
		},
	}

	router := dsr.NewRouter(
		dsr.WithGitDriver(repoDriver),
	)

	packDir := t.TempDir()
	packPath := filepath.Join(packDir, "pack.yaml")
	if err := os.WriteFile(packPath, []byte(`
name: stable-pack
repos:
  - org: org
    name: stable-repo
`), 0o644); err != nil {
		t.Fatal(err)
	}

	capturer := dsr.NewCapturer(router, nil)
	cfg := calibrate.CaptureConfig{
		Schematic:  "gnd",
		SourcePack: packPath,
	}

	// Run 1
	out1 := t.TempDir()
	cfg.OutputDir = out1
	if err := capturer.Capture(context.Background(), cfg); err != nil {
		t.Fatalf("capture 1: %v", err)
	}

	// Run 2
	out2 := t.TempDir()
	cfg.OutputDir = out2
	if err := capturer.Capture(context.Background(), cfg); err != nil {
		t.Fatalf("capture 2: %v", err)
	}

	m1, _ := calibrate.ReadManifest(os.DirFS(out1))
	m2, _ := calibrate.ReadManifest(os.DirFS(out2))

	if len(m1.Repos) != len(m2.Repos) {
		t.Fatalf("repo count differs: %d vs %d", len(m1.Repos), len(m2.Repos))
	}
	if m1.Repos[0].SHA != m2.Repos[0].SHA {
		t.Errorf("repo SHA differs: %q vs %q", m1.Repos[0].SHA, m2.Repos[0].SHA)
	}
}
