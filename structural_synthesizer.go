package dsr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

// StructuralSynthesizer produces DerivedArtifacts by deterministically
// merging source metadata. No LLM calls — output is a structured Markdown
// document with component maps, source indexes, and version information.
type StructuralSynthesizer struct {
	Summarizer Summarizer
}

var _ Synthesizer = (*StructuralSynthesizer)(nil)

const (
	SectionComponentMap = "component-map"
	SectionSourceIndex  = "source-index"
	SectionVersionInfo  = "version-info"

	charsPerToken = 4
)

var defaultSections = []string{SectionComponentMap, SectionSourceIndex, SectionVersionInfo}

var sectionTitles = map[string]string{
	SectionComponentMap: "Component Map",
	SectionSourceIndex:  "Source Index",
	SectionVersionInfo:  "Version Information",
}

func (ss *StructuralSynthesizer) Synthesize(
	ctx context.Context,
	pack *SourcePack,
	reader toolkit.SourceReader,
	opts SynthesisOpts,
) (*DerivedArtifact, error) {
	if pack == nil {
		return nil, fmt.Errorf("synthesize: nil source pack")
	}

	sources := pack.ToSources(opts.Attrs)
	sections := opts.Sections
	if len(sections) == 0 {
		sections = defaultSections
	}

	sectionSet := make(map[string]bool, len(sections))
	for _, s := range sections {
		sectionSet[s] = true
	}

	result := &DerivedArtifact{
		Kind:        "domain-context",
		SourcePack:  pack.Name,
		Sections:    make(map[string]string, len(sections)),
		GeneratedAt: time.Now(),
	}

	for _, src := range sources {
		result.Sources = append(result.Sources, src.Name)
	}

	if sectionSet[SectionComponentMap] {
		result.Sections[SectionComponentMap] = ss.buildComponentMap(sources)
	}
	if sectionSet[SectionSourceIndex] {
		result.Sections[SectionSourceIndex] = ss.buildSourceIndex(sources)
	}
	if sectionSet[SectionVersionInfo] {
		result.Sections[SectionVersionInfo] = ss.buildVersionInfo(pack, opts.Attrs)
	}

	content := ss.assembleContent(pack, result.Sections, sections)
	if opts.TokenBudget > 0 {
		content = ss.fitBudget(content, opts.TokenBudget)
	}
	result.Content = content

	return result, nil
}

func (ss *StructuralSynthesizer) buildComponentMap(sources []toolkit.Source) string {
	var b strings.Builder
	b.WriteString("| Component | Org | Purpose | Branch |\n")
	b.WriteString("|-----------|-----|---------|--------|\n")
	for _, src := range sources {
		if src.Kind != toolkit.SourceKindRepo {
			continue
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			src.Name, src.Org, src.Purpose, src.Branch))
	}
	return b.String()
}

func (ss *StructuralSynthesizer) buildSourceIndex(sources []toolkit.Source) string {
	var b strings.Builder

	var repos, docs []toolkit.Source
	for _, s := range sources {
		switch s.Kind {
		case toolkit.SourceKindRepo:
			repos = append(repos, s)
		case toolkit.SourceKindDoc:
			docs = append(docs, s)
		default:
			docs = append(docs, s)
		}
	}

	if len(repos) > 0 {
		b.WriteString("### Repositories\n\n")
		for i, s := range repos {
			b.WriteString(fmt.Sprintf("%d. **%s** — %s (`%s`)\n", i+1, s.Name, s.Purpose, s.URI))
		}
		b.WriteString("\n")
	}

	if len(docs) > 0 {
		b.WriteString("### Documentation\n\n")
		for i, s := range docs {
			b.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, s.Name, s.URI))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (ss *StructuralSynthesizer) buildVersionInfo(pack *SourcePack, attrs map[string]string) string {
	if pack.VersionKey == "" {
		return "No version key configured.\n"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("- **Version key:** `%s`\n", pack.VersionKey))
	if v, ok := attrs[pack.VersionKey]; ok {
		b.WriteString(fmt.Sprintf("- **Resolved version:** %s\n", v))
	} else {
		b.WriteString("- **Resolved version:** (not provided)\n")
	}
	for _, r := range pack.Repos {
		if r.BranchPattern != "" {
			resolved := ResolveBranch(r.BranchPattern, attrs)
			b.WriteString(fmt.Sprintf("- %s/%s: `%s` → `%s`\n", r.Org, r.Name, r.BranchPattern, resolved))
		}
	}
	return b.String()
}

func (ss *StructuralSynthesizer) assembleContent(
	pack *SourcePack,
	sections map[string]string,
	order []string,
) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s — Domain Context\n\n", pack.Name))
	if pack.Description != "" {
		b.WriteString(pack.Description)
		b.WriteString("\n\n")
	}

	for _, key := range order {
		content, ok := sections[key]
		if !ok || content == "" {
			continue
		}
		title := sectionTitles[key]
		if title == "" {
			title = key
		}
		b.WriteString(fmt.Sprintf("## %s\n\n", title))
		b.WriteString(content)
		b.WriteString("\n")
	}

	return b.String()
}

func (ss *StructuralSynthesizer) fitBudget(content string, tokenBudget int) string {
	if ss.Summarizer != nil {
		return ss.Summarizer.Summarize(content, tokenBudget, StrategyFull)
	}
	charBudget := tokenBudget * charsPerToken
	if len(content) <= charBudget {
		return content
	}
	return content[:charBudget] + "\n... [truncated to fit token budget]"
}
