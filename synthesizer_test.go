package dsr

import (
	"context"
	"strings"
	"testing"

	"github.com/dpopsuev/origami/toolkit"
)

type stubReader struct{}

func (stubReader) Ensure(_ context.Context, _ toolkit.Source) error { return nil }
func (stubReader) Search(_ context.Context, _ toolkit.Source, _ string, _ int) ([]toolkit.SearchResult, error) {
	return nil, nil
}
func (stubReader) Read(_ context.Context, _ toolkit.Source, _ string) ([]byte, error) {
	return nil, nil
}
func (stubReader) List(_ context.Context, _ toolkit.Source, _ string, _ int) ([]toolkit.ContentEntry, error) {
	return nil, nil
}

func ptpTestPack() *SourcePack {
	return &SourcePack{
		Name:        "ptp",
		Domain:      "PTP Operator",
		Description: "PTP Operator sources for root-cause analysis",
		VersionKey:  "ocp_version",
		Repos: []SourcePackRepo{
			{Org: "openshift", Name: "linuxptp-daemon", Purpose: "PTP daemon", BranchPattern: "release-{ocp_version}"},
			{Org: "openshift", Name: "ptp-operator", Purpose: "Operator lifecycle", BranchPattern: "release-{ocp_version}"},
			{Org: "openshift-kni", Name: "cnf-features-deploy", Purpose: "Test framework", BranchPattern: "master"},
			{Org: "redhat-cne", Name: "cloud-event-proxy", Purpose: "Event sidecar", BranchPattern: "release-{ocp_version}"},
		},
		Docs: []string{
			"https://docs.openshift.com/container-platform/latest/networking/ptp/about-ptp.html",
			"https://docs.openshift.com/container-platform/latest/networking/ptp/configuring-ptp.html",
		},
	}
}

func TestStructuralSynthesizer_Synthesize(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		Attrs: map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if art.Kind != "domain-context" {
		t.Errorf("kind = %q, want domain-context", art.Kind)
	}
	if art.SourcePack != "ptp" {
		t.Errorf("source_pack = %q, want ptp", art.SourcePack)
	}
	if len(art.Sources) != 6 {
		t.Errorf("sources count = %d, want 6 (4 repos + 2 docs)", len(art.Sources))
	}

	if _, ok := art.Sections[SectionComponentMap]; !ok {
		t.Error("missing component-map section")
	}
	if _, ok := art.Sections[SectionSourceIndex]; !ok {
		t.Error("missing source-index section")
	}
	if _, ok := art.Sections[SectionVersionInfo]; !ok {
		t.Error("missing version-info section")
	}

	if !strings.Contains(art.Content, "# ptp — Domain Context") {
		t.Error("content missing title")
	}
	if !strings.Contains(art.Content, "PTP Operator sources") {
		t.Error("content missing description")
	}
}

func TestStructuralSynthesizer_ComponentMap(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		Sections: []string{SectionComponentMap},
		Attrs:    map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cm := art.Sections[SectionComponentMap]
	if !strings.Contains(cm, "linuxptp-daemon") {
		t.Error("component map missing linuxptp-daemon")
	}
	if !strings.Contains(cm, "openshift") {
		t.Error("component map missing org")
	}
	if !strings.Contains(cm, "release-4.21") {
		t.Error("component map missing resolved branch")
	}
}

func TestStructuralSynthesizer_SourceIndex(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		Sections: []string{SectionSourceIndex},
		Attrs:    map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	si := art.Sections[SectionSourceIndex]
	if !strings.Contains(si, "### Repositories") {
		t.Error("source index missing Repositories heading")
	}
	if !strings.Contains(si, "### Documentation") {
		t.Error("source index missing Documentation heading")
	}
	if !strings.Contains(si, "about-ptp.html") {
		t.Error("source index missing doc URL")
	}
}

func TestStructuralSynthesizer_VersionInfo(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		Sections: []string{SectionVersionInfo},
		Attrs:    map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vi := art.Sections[SectionVersionInfo]
	if !strings.Contains(vi, "ocp_version") {
		t.Error("version info missing version key")
	}
	if !strings.Contains(vi, "4.21") {
		t.Error("version info missing resolved version")
	}
}

func TestStructuralSynthesizer_TokenBudget(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		TokenBudget: 50,
		Attrs:       map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	charBudget := 50 * 4
	if len(art.Content) > charBudget+50 { // allow for truncation message
		t.Errorf("content length %d exceeds budget %d chars", len(art.Content), charBudget)
	}
	if !strings.Contains(art.Content, "truncated") {
		t.Error("truncated content should contain truncation notice")
	}
}

func TestStructuralSynthesizer_TokenBudget_WithSummarizer(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{Summarizer: TruncateSummarizer{}}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		TokenBudget: 50,
		Attrs:       map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	charBudget := 50 * 4
	if len(art.Content) > charBudget+50 {
		t.Errorf("content length %d exceeds budget %d chars", len(art.Content), charBudget)
	}
}

func TestStructuralSynthesizer_NilPack(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	_, err := ss.Synthesize(context.Background(), nil, stubReader{}, SynthesisOpts{})
	if err == nil {
		t.Error("expected error for nil pack")
	}
}

func TestStructuralSynthesizer_EmptyPack(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := &SourcePack{Name: "empty"}
	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, SynthesisOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if art.Kind != "domain-context" {
		t.Errorf("kind = %q, want domain-context", art.Kind)
	}
	if len(art.Sources) != 0 {
		t.Errorf("sources count = %d, want 0", len(art.Sources))
	}
}

func TestStructuralSynthesizer_NilReader(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	art, err := ss.Synthesize(context.Background(), pack, nil, SynthesisOpts{
		Attrs: map[string]string{"ocp_version": "4.21"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if art.Kind != "domain-context" {
		t.Errorf("kind = %q, want domain-context", art.Kind)
	}
}

func TestStructuralSynthesizer_SelectSections(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := ptpTestPack()
	opts := SynthesisOpts{
		Sections: []string{SectionComponentMap},
		Attrs:    map[string]string{"ocp_version": "4.21"},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := art.Sections[SectionComponentMap]; !ok {
		t.Error("missing requested component-map section")
	}
	if _, ok := art.Sections[SectionSourceIndex]; ok {
		t.Error("source-index section should not be present when not requested")
	}
	if _, ok := art.Sections[SectionVersionInfo]; ok {
		t.Error("version-info section should not be present when not requested")
	}
}

func TestStructuralSynthesizer_NoVersionKey(t *testing.T) {
	t.Parallel()
	ss := &StructuralSynthesizer{}
	pack := &SourcePack{
		Name: "simple",
		Repos: []SourcePackRepo{
			{Org: "example", Name: "repo-a", Purpose: "test repo"},
		},
	}

	art, err := ss.Synthesize(context.Background(), pack, stubReader{}, SynthesisOpts{
		Sections: []string{SectionVersionInfo},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	vi := art.Sections[SectionVersionInfo]
	if !strings.Contains(vi, "No version key") {
		t.Error("version info should indicate no version key configured")
	}
}
