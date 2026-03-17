package dsr_test

import (
	"testing"

	"github.com/dpopsuev/rh-dsr"
	"github.com/dpopsuev/origami/schematics/toolkit"
)

func TestVersionMatrix_ExactMatch(t *testing.T) {
	vm := dsr.NewVersionMatrix(
		dsr.VersionEntry{Version: "4.18", Branch: "release-4.18", DocsURL: "https://docs.example.com/4.18"},
		dsr.VersionEntry{Version: "4.21", Branch: "release-4.21", DocsURL: "https://docs.example.com/4.21"},
	)

	entry := vm.Resolve("4.21")
	if entry == nil {
		t.Fatal("expected entry for 4.21")
	}
	if entry.Branch != "release-4.21" {
		t.Errorf("branch = %q, want release-4.21", entry.Branch)
	}
	if entry.DocsURL != "https://docs.example.com/4.21" {
		t.Errorf("docs_url = %q, want https://docs.example.com/4.21", entry.DocsURL)
	}
}

func TestVersionMatrix_PrefixMatch(t *testing.T) {
	vm := dsr.NewVersionMatrix(
		dsr.VersionEntry{Version: "4.21", Branch: "release-4.21"},
	)

	entry := vm.Resolve("4.21.3")
	if entry == nil {
		t.Fatal("expected prefix match for 4.21.3")
	}
	if entry.Branch != "release-4.21" {
		t.Errorf("branch = %q, want release-4.21", entry.Branch)
	}
}

func TestVersionMatrix_NoMatch(t *testing.T) {
	vm := dsr.NewVersionMatrix(
		dsr.VersionEntry{Version: "4.18", Branch: "release-4.18"},
	)

	if entry := vm.Resolve("4.21"); entry != nil {
		t.Errorf("expected nil for unmatched version, got %+v", entry)
	}
}

func TestVersionMatrix_NilMatrix(t *testing.T) {
	var vm *dsr.VersionMatrix
	if entry := vm.Resolve("4.21"); entry != nil {
		t.Errorf("expected nil from nil matrix, got %+v", entry)
	}
}

func TestVersionMatrix_ResolveBranch_Fallback(t *testing.T) {
	vm := dsr.NewVersionMatrix(
		dsr.VersionEntry{Version: "4.21", Branch: "release-4.21"},
	)

	got := vm.ResolveBranch("4.99", "main")
	if got != "main" {
		t.Errorf("ResolveBranch fallback = %q, want main", got)
	}

	got = vm.ResolveBranch("4.21", "main")
	if got != "release-4.21" {
		t.Errorf("ResolveBranch = %q, want release-4.21", got)
	}
}

func TestVersionMatrix_ResolveSource(t *testing.T) {
	vm := dsr.NewVersionMatrix(
		dsr.VersionEntry{Version: "4.21", Branch: "release-4.21"},
	)

	src := toolkit.Source{
		Name:   "ptp-operator",
		Kind:   toolkit.SourceKindRepo,
		Branch: "main",
	}

	resolved := vm.ResolveSource(src, "4.21")
	if resolved.Branch != "release-4.21" {
		t.Errorf("resolved branch = %q, want release-4.21", resolved.Branch)
	}
	if src.Branch != "main" {
		t.Error("original source was mutated")
	}

	unresolved := vm.ResolveSource(src, "4.99")
	if unresolved.Branch != "main" {
		t.Errorf("unresolved branch = %q, want main (unchanged)", unresolved.Branch)
	}
}
