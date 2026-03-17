package dsr

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

func testdataResolver(name string) (string, error) {
	path := testdataPath(fmt.Sprintf("pack-%s.yaml", name))
	return path, nil
}

func TestLoadPack_Simple(t *testing.T) {
	pack, err := LoadPack(testdataPath("pack-ocp-platform.yaml"), nil)
	if err != nil {
		t.Fatalf("LoadPack: %v", err)
	}
	if pack.Name != "ocp-platform" {
		t.Errorf("name = %q, want ocp-platform", pack.Name)
	}
	if len(pack.Repos) != 1 {
		t.Fatalf("repos = %d, want 1", len(pack.Repos))
	}
	if pack.Repos[0].Org != "openshift" || pack.Repos[0].Name != "cluster-etcd-operator" {
		t.Errorf("repo = %+v", pack.Repos[0])
	}
	if len(pack.Docs) != 1 {
		t.Fatalf("docs = %d, want 1", len(pack.Docs))
	}
}

func TestLoadPack_WithIncludes(t *testing.T) {
	pack, err := LoadPack(testdataPath("pack-ptp.yaml"), testdataResolver)
	if err != nil {
		t.Fatalf("LoadPack: %v", err)
	}
	if pack.Name != "ptp" {
		t.Errorf("name = %q, want ptp", pack.Name)
	}
	// 3 PTP repos + 1 from ocp-platform = 4 total (no dedup needed here)
	if len(pack.Repos) != 4 {
		t.Fatalf("repos = %d, want 4", len(pack.Repos))
	}
	// Docs: ocp-architecture.md + ptp-architecture.md
	if len(pack.Docs) != 2 {
		t.Fatalf("docs = %d, want 2", len(pack.Docs))
	}
}

func TestLoadPack_Dedup(t *testing.T) {
	base := &SourcePack{
		Repos: []SourcePackRepo{
			{Org: "openshift", Name: "ptp-operator", Purpose: "old purpose"},
		},
	}
	overlay := &SourcePack{
		Repos: []SourcePackRepo{
			{Org: "openshift", Name: "ptp-operator", Purpose: "new purpose"},
			{Org: "openshift", Name: "new-repo", Purpose: "new repo"},
		},
	}
	merged := MergePacks(base, overlay)
	if len(merged.Repos) != 2 {
		t.Fatalf("repos = %d, want 2", len(merged.Repos))
	}
	if merged.Repos[0].Purpose != "new purpose" {
		t.Errorf("dedup should use last-wins: got %q", merged.Repos[0].Purpose)
	}
}

func TestLoadPack_CycleDetection(t *testing.T) {
	_, err := LoadPack(testdataPath("pack-cycle-a.yaml"), testdataResolver)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestSourcePack_ToSources(t *testing.T) {
	pack := &SourcePack{
		Name:       "ptp",
		Domain:     "PTP Operator",
		VersionKey: "ocp_version",
		Repos: []SourcePackRepo{
			{Org: "openshift", Name: "ptp-operator", Purpose: "SUT", BranchPattern: "release-{ocp_version}"},
			{Org: "openshift-kni", Name: "cnf-features-deploy", Purpose: "test", BranchPattern: "master"},
		},
		Docs: []string{"docs/ptp.md"},
	}

	attrs := map[string]string{"ocp_version": "4.21"}
	sources := pack.ToSources(attrs)

	if len(sources) != 3 {
		t.Fatalf("sources = %d, want 3 (2 repos + 1 doc)", len(sources))
	}

	if sources[0].Branch != "release-4.21" {
		t.Errorf("branch = %q, want release-4.21", sources[0].Branch)
	}
	if sources[0].Org != "openshift" {
		t.Errorf("org = %q, want openshift", sources[0].Org)
	}
	if sources[0].Tags["domain"] != "PTP Operator" {
		t.Errorf("domain tag = %q", sources[0].Tags["domain"])
	}
	if sources[1].Branch != "master" {
		t.Errorf("branch = %q, want master", sources[1].Branch)
	}
	if sources[2].Kind != toolkit.SourceKindDoc {
		t.Errorf("doc kind = %q", sources[2].Kind)
	}
}
