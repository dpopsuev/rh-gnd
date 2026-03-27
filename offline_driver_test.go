package dsr_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/dpopsuev/origami-gnd"
	"github.com/dpopsuev/origami/toolkit"
)

func offlineBundle() fstest.MapFS {
	return fstest.MapFS{
		"repos/linuxptp-daemon/cmd/main.go":     {Data: []byte("package main\nfunc main() { startDaemon() }\n")},
		"repos/linuxptp-daemon/pkg/ptp/sync.go": {Data: []byte("package ptp\nfunc SyncClock() error { return nil }\n")},
		"repos/ptp-operator/api/v1/types.go":    {Data: []byte("package v1\ntype PtpConfig struct { Profile string }\n")},
		"docs/ptp/architecture.md":              {Data: []byte("# PTP Architecture\n\nlinuxptp-daemon runs ptp4l and phc2sys.\n")},
	}
}

func TestOfflineFSDriver_Read(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindRepo)

	src := toolkit.Source{Name: "linuxptp-daemon", Kind: toolkit.SourceKindRepo}
	data, err := driver.Read(ctx, src, "cmd/main.go")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got := string(data); got != "package main\nfunc main() { startDaemon() }\n" {
		t.Errorf("unexpected content: %q", got)
	}
}

func TestOfflineFSDriver_Read_NotFound(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindRepo)

	src := toolkit.Source{Name: "linuxptp-daemon", Kind: toolkit.SourceKindRepo}
	_, err := driver.Read(ctx, src, "nonexistent.go")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestOfflineFSDriver_Search(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindRepo)

	src := toolkit.Source{Name: "linuxptp-daemon", Kind: toolkit.SourceKindRepo}
	results, err := driver.Search(ctx, src, "SyncClock", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Path != "pkg/ptp/sync.go" {
		t.Errorf("path = %q, want pkg/ptp/sync.go", results[0].Path)
	}
}

func TestOfflineFSDriver_Search_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindRepo)

	src := toolkit.Source{Name: "linuxptp-daemon", Kind: toolkit.SourceKindRepo}
	results, err := driver.Search(ctx, src, "syncclock", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestOfflineFSDriver_List(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindRepo)

	src := toolkit.Source{Name: "linuxptp-daemon", Kind: toolkit.SourceKindRepo}
	entries, err := driver.List(ctx, src, ".", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}
	var found bool
	for _, e := range entries {
		if e.Path == "cmd" && e.IsDir {
			found = true
		}
	}
	if !found {
		t.Errorf("expected cmd/ directory in listing, got: %v", entries)
	}
}

func TestOfflineFSDriver_Ensure_NoOp(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindRepo)

	src := toolkit.Source{Name: "linuxptp-daemon", Kind: toolkit.SourceKindRepo}
	if err := driver.Ensure(ctx, src); err != nil {
		t.Fatalf("Ensure should be no-op: %v", err)
	}
}

func TestOfflineFSDriver_DocKind(t *testing.T) {
	ctx := context.Background()
	bundle := offlineBundle()
	driver := dsr.NewOfflineFSDriver(bundle, toolkit.SourceKindDoc)

	src := toolkit.Source{
		Name:      "architecture.md",
		Kind:      toolkit.SourceKindDoc,
		LocalPath: "docs/ptp/architecture.md",
	}
	data, err := driver.Read(ctx, src, "architecture.md")
	if err != nil {
		t.Fatalf("Read doc: %v", err)
	}
	if got := string(data); got == "" {
		t.Error("expected non-empty doc content")
	}
}

func TestOfflineFSDriver_Handles(t *testing.T) {
	driver := dsr.NewOfflineFSDriver(nil, toolkit.SourceKindRepo)
	if got := driver.Handles(); got != toolkit.SourceKindRepo {
		t.Errorf("Handles() = %q, want %q", got, toolkit.SourceKindRepo)
	}
}
