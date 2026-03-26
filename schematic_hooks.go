package dsr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/dpopsuev/origami/agentport"
	"github.com/dpopsuev/origami/circuit"
	"github.com/dpopsuev/origami/engine"
	"github.com/dpopsuev/origami/toolkit"
)

// Hooks returns the SessionHooks that fold-generated code calls.
func Hooks() engine.SessionHooks {
	return engine.SessionHooks{
		CreateSession: createSession,
		StepSchemas: []engine.StepSchema{
			{
				Name: "synthesize",
				Defs: []toolkit.FieldDef{
					{Name: "summary", Type: "string", Required: true, Desc: "Cohesive summary of gathered code context"},
					{Name: "key_findings", Type: "array", Required: false, Desc: "Key findings from code analysis"},
				},
			},
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

func createSession(_ context.Context, params engine.SessionParams) (*engine.SessionConfig, error) {
	extra := params.Extra

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

	// Extract source catalog from forwarded context.
	var catalog toolkit.SourceCatalog
	if sources, ok := extra["params.sources"]; ok {
		if sc, ok := sources.(toolkit.SourceCatalog); ok {
			catalog = sc
		}
	}

	// Create reader with default drivers.
	reader := NewRouter()
	if catalog == nil {
		catalog = &toolkit.SliceCatalog{}
	}

	// Determine backend: stub (deterministic) or llm (dispatched synthesis).
	backendStr, _ := extra["backend"].(string)
	var synthDisp agentport.Dispatcher
	if backendStr == "llm" && params.Dispatcher != nil {
		synthDisp = params.Dispatcher
	}

	def, err := circuit.LoadCircuit(DefaultCircuitYAML())
	if err != nil {
		return nil, fmt.Errorf("load gnd circuit: %w", err)
	}

	gatherComp := TransformerComponent(reader, catalog)
	synthComp := SynthesizeComponent(synthDisp)

	// Build walker context with search keywords and forwarded extra.
	walkerCtx := map[string]any{}
	if len(searchKeywords) > 0 {
		walkerCtx["dsr.search_keywords"] = searchKeywords
	}
	for k, v := range extra {
		if k == "backend" || k == "search_keywords" {
			continue
		}
		walkerCtx[k] = v
	}

	runFunc := func(ctx context.Context) (any, error) {
		results := engine.BatchWalk(ctx, engine.BatchWalkConfig{
			Def: def,
			Shared: engine.GraphRegistries{
				Transformers: engine.TransformerRegistry{},
			},
			Cases: []engine.BatchCase{
				{
					ID:         "gnd-0",
					Context:    walkerCtx,
					Components: []*engine.Component{gatherComp, synthComp},
				},
			},
			Parallel: 1,
			Observer: params.Observer,
		})

		if len(results) == 0 {
			return nil, fmt.Errorf("gnd: no results from BatchWalk")
		}
		r := results[0]
		if r.Error != nil {
			return nil, fmt.Errorf("gnd walk: %w", r.Error)
		}

		if art, ok := r.StepArtifacts["synthesize"]; ok {
			return art.Raw(), nil
		}
		if art, ok := r.StepArtifacts["read"]; ok {
			return art.Raw(), nil
		}
		return nil, fmt.Errorf("gnd: no output from synthesize or read node")
	}

	slog.Info("gnd session created",
		"keywords", len(searchKeywords),
		"sources", len(catalog.Sources()),
		"backend", backendStr)

	return &engine.SessionConfig{
		CircuitDef: def,
		Meta: engine.SessionMeta{
			TotalCases: 1,
			Scenario:   "gnd",
		},
		RunFunc: runFunc,
	}, nil
}
