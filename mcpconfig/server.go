package mcpconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	framework "github.com/dpopsuev/origami"
	"github.com/dpopsuev/origami/dispatch"
	fwmcp "github.com/dpopsuev/origami/mcp"
	dsr "github.com/dpopsuev/rh-gnd"
	"github.com/dpopsuev/origami/schematics/toolkit"
)

// Server wraps the generic CircuitServer with GND-specific domain hooks.
type Server struct {
	*fwmcp.CircuitServer
	Reader  toolkit.SourceReader
	Catalog toolkit.SourceCatalog
}

// ServerOption configures a GND MCP server.
type ServerOption func(*Server)

// WithReader injects a SourceReader for code access during gathering.
func WithReader(r toolkit.SourceReader) ServerOption {
	return func(s *Server) { s.Reader = r }
}

// WithCatalog injects a SourceCatalog listing available sources.
func WithCatalog(c toolkit.SourceCatalog) ServerOption {
	return func(s *Server) { s.Catalog = c }
}

// NewServer creates a GND MCP server.
func NewServer(opts ...ServerOption) *Server {
	s := &Server{}
	for _, opt := range opts {
		opt(s)
	}
	s.CircuitServer = fwmcp.NewCircuitServer(s.buildConfig())
	return s
}

func (s *Server) buildConfig() fwmcp.CircuitConfig {
	return fwmcp.CircuitConfig{
		Name:    "origami-gnd",
		Version: "dev",
		StepSchemas: []fwmcp.StepSchema{
			{
				Name: "synthesize",
				Fields: map[string]string{
					"summary":      "string",
					"key_findings": "array",
				},
				Defs: []fwmcp.FieldDef{
					{Name: "summary", Type: "string", Required: true, Desc: "Cohesive summary of gathered code context"},
					{Name: "key_findings", Type: "array", Required: false, Desc: "Key findings from code analysis"},
				},
			},
		},
		ExtraParamDefs: []fwmcp.ExtraParamDef{
			{Name: "search_keywords", Type: "object", Description: "Search keywords for code gathering ([]string)"},
			{Name: "sources", Type: "object", Description: "Source catalog entries forwarded by mediator"},
			{Name: "backend", Type: "string", Description: "Transformer backend", Enum: []string{"stub", "llm"}},
		},
		WorkerPreamble:            "You are a GND (Gather & Diffuse) code context worker.",
		DefaultGetNextStepTimeout: 10000,
		DefaultSessionTTL:         300000,
		CreateSession: func(ctx context.Context, params fwmcp.StartParams, disp *dispatch.MuxDispatcher, bus *dispatch.SignalBus) (fwmcp.RunFunc, fwmcp.SessionMeta, error) {
			return s.createSession(ctx, params, disp, bus)
		},
		FormatReport: func(result any) (string, any, error) {
			data, err := json.Marshal(result)
			if err != nil {
				return "", nil, fmt.Errorf("marshal gnd result: %w", err)
			}
			return string(data), result, nil
		},
	}
}

func (s *Server) createSession(_ context.Context, params fwmcp.StartParams, disp *dispatch.MuxDispatcher, _ *dispatch.SignalBus) (fwmcp.RunFunc, fwmcp.SessionMeta, error) {
	extra := params.Extra

	// Resolve reader and catalog — use injected defaults, allow override from extra.
	reader := s.Reader
	catalog := s.Catalog

	// Extract search keywords from forwarded context.
	var searchKeywords []string
	if kw, ok := extra["dsr.search_keywords"].([]any); ok {
		for _, v := range kw {
			if s, ok := v.(string); ok {
				searchKeywords = append(searchKeywords, s)
			}
		}
	}
	if kw, ok := extra["search_keywords"].([]any); ok && len(searchKeywords) == 0 {
		for _, v := range kw {
			if s, ok := v.(string); ok {
				searchKeywords = append(searchKeywords, s)
			}
		}
	}

	// Extract source catalog from forwarded context if available.
	if sources, ok := extra["params.sources"]; ok && catalog == nil {
		if sc, ok := sources.(toolkit.SourceCatalog); ok {
			catalog = sc
		}
	}

	if reader == nil {
		slog.Warn("gnd: no SourceReader configured, gather phase will produce empty results")
		reader = dsr.NewRouter()
	}
	if catalog == nil {
		catalog = &toolkit.SliceCatalog{}
	}

	// Determine backend: stub (deterministic passthrough) or llm (dispatched).
	backendStr, _ := extra["backend"].(string)
	var synthDisp dispatch.Dispatcher
	if backendStr == "llm" {
		synthDisp = disp
	}

	def, err := framework.LoadCircuit(dsr.DefaultCircuitYAML())
	if err != nil {
		return nil, fwmcp.SessionMeta{}, fmt.Errorf("load gnd circuit: %w", err)
	}

	gatherComp := dsr.TransformerComponent(reader, catalog)
	synthComp := dsr.SynthesizeComponent(synthDisp)

	walkerCtx := map[string]any{}
	if len(searchKeywords) > 0 {
		walkerCtx["dsr.search_keywords"] = searchKeywords
	}
	// Forward all extra context keys for downstream transformers.
	for k, v := range extra {
		if k == "backend" || k == "search_keywords" {
			continue
		}
		walkerCtx[k] = v
	}

	runFn := func(ctx context.Context) (any, error) {
		results := framework.BatchWalk(ctx, framework.BatchWalkConfig{
			Def: def,
			Shared: framework.GraphRegistries{
				Transformers: framework.TransformerRegistry{},
			},
			Cases: []framework.BatchCase{
				{
					ID:         "gnd-0",
					Context:    walkerCtx,
					Components: []*framework.Component{gatherComp, synthComp},
				},
			},
			Parallel: 1,
		})

		if len(results) == 0 {
			return nil, fmt.Errorf("gnd: no results from BatchWalk")
		}
		r := results[0]
		if r.Error != nil {
			return nil, fmt.Errorf("gnd walk: %w", r.Error)
		}

		// Return the synthesize node's artifact.
		if art, ok := r.StepArtifacts["synthesize"]; ok {
			return art.Raw(), nil
		}
		// Fall back to read node output.
		if art, ok := r.StepArtifacts["read"]; ok {
			return art.Raw(), nil
		}
		return nil, fmt.Errorf("gnd: no output from synthesize or read node")
	}

	meta := fwmcp.SessionMeta{
		TotalCases: 1,
		Scenario:   "gnd",
	}

	return runFn, meta, nil
}
