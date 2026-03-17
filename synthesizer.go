package dsr

import (
	"context"
	"time"

	"github.com/dpopsuev/origami/schematics/toolkit"
)

// DerivedArtifact is a transient domain context document produced by
// combining multiple resolved sources. Never persisted to git.
type DerivedArtifact struct {
	Kind        string            `json:"kind"`
	SourcePack  string            `json:"source_pack"`
	Sources     []string          `json:"sources"`
	Content     string            `json:"content"`
	Sections    map[string]string `json:"sections"`
	GeneratedAt time.Time         `json:"generated_at"`
}

// SynthesisOpts controls what a Synthesizer produces.
type SynthesisOpts struct {
	TokenBudget int
	Sections    []string
	Attrs       map[string]string
}

// Synthesizer combines multiple resolved sources into a DerivedArtifact.
type Synthesizer interface {
	Synthesize(ctx context.Context, pack *SourcePack,
		reader toolkit.SourceReader, opts SynthesisOpts) (*DerivedArtifact, error)
}
