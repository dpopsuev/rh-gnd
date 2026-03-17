package dsr

import "github.com/dpopsuev/origami/schematics/toolkit"

// RouteRequest describes what the circuit is looking for — a component under
// investigation, a hypothesis being tested, the current step, and any
// additional tag constraints.
type RouteRequest struct {
	Component  string
	Hypothesis string
	Step       string
	Tags       map[string]string
}

// RouteRule decides whether a source is relevant for a given request.
type RouteRule interface {
	Match(source toolkit.Source, req RouteRequest) bool
}

// SourceRouter selects sources from a catalog based on configurable
// rules. If no rule matches any source, all sources are returned (safe default).
type SourceRouter struct {
	catalog toolkit.SourceCatalog
	rules   []RouteRule
}

// NewSourceRouter creates a router over the given catalog with the provided rules.
func NewSourceRouter(catalog toolkit.SourceCatalog, rules ...RouteRule) *SourceRouter {
	return &SourceRouter{catalog: catalog, rules: rules}
}

// Route returns sources where at least one rule matches. Sources with
// ReadPolicy == ReadAlways are always included regardless of rule matching.
// If no rules are configured or no conditional rule matches any source,
// all sources are returned.
func (r *SourceRouter) Route(req RouteRequest) []toolkit.Source {
	if len(r.rules) == 0 || r.catalog == nil {
		return r.allSources()
	}

	sources := r.catalog.Sources()
	seen := make(map[string]bool, len(sources))
	matched := make([]toolkit.Source, 0, len(sources))

	for _, src := range sources {
		if src.IsAlwaysRead() {
			seen[src.Name] = true
			matched = append(matched, src)
			continue
		}
		for _, rule := range r.rules {
			if rule.Match(src, req) {
				if !seen[src.Name] {
					seen[src.Name] = true
					matched = append(matched, src)
				}
				break
			}
		}
	}

	alwaysCount := 0
	for _, src := range matched {
		if src.IsAlwaysRead() {
			alwaysCount++
		}
	}

	if len(matched) == alwaysCount {
		return r.allSources()
	}
	return matched
}

func (r *SourceRouter) allSources() []toolkit.Source {
	if r.catalog == nil {
		return nil
	}
	sources := r.catalog.Sources()
	out := make([]toolkit.Source, len(sources))
	copy(out, sources)
	return out
}

// TagMatchRule matches sources whose Tags contain all the key-value pairs
// specified in the rule's Required map.
type TagMatchRule struct {
	Required map[string]string
}

// Match returns true if source.Tags[k] == v for every (k, v) in Required.
func (r TagMatchRule) Match(src toolkit.Source, _ RouteRequest) bool {
	for k, v := range r.Required {
		if src.Tags[k] != v {
			return false
		}
	}
	return true
}

// RequestTagMatchRule matches sources whose Tags overlap with the request's
// Tags. For every key present in both source.Tags and req.Tags, the values
// must be equal.
type RequestTagMatchRule struct{}

// Match returns true if all overlapping tag keys have equal values.
// Sources with no tags always match (no constraints to violate).
func (RequestTagMatchRule) Match(src toolkit.Source, req RouteRequest) bool {
	if len(src.Tags) == 0 || len(req.Tags) == 0 {
		return true
	}
	for k, rv := range req.Tags {
		if sv, ok := src.Tags[k]; ok && sv != rv {
			return false
		}
	}
	return true
}

// Layer tag constants for tag-based source layering.
const (
	LayerTagKey        = "layer"
	LayerBase          = "base"
	LayerVersion       = "version"
	LayerInvestigation = "investigation"
)

// LayeredRoute composes three tag queries (base, version, investigation)
// and returns their union, deduplicated by source name. Always-read sources
// are included regardless.
func (r *SourceRouter) LayeredRoute(baseTags, versionTags, investigationTags map[string]string) []toolkit.Source {
	if r.catalog == nil {
		return nil
	}

	sources := r.catalog.Sources()
	layers := []map[string]string{baseTags, versionTags, investigationTags}
	seen := make(map[string]bool, len(sources))
	var result []toolkit.Source

	for _, src := range sources {
		if src.IsAlwaysRead() && !seen[src.Name] {
			seen[src.Name] = true
			result = append(result, src)
		}
	}

	for _, tags := range layers {
		if len(tags) == 0 {
			continue
		}
		rule := TagMatchRule{Required: tags}
		for _, src := range sources {
			if seen[src.Name] {
				continue
			}
			if rule.Match(src, RouteRequest{}) {
				seen[src.Name] = true
				result = append(result, src)
			}
		}
	}

	if len(result) == 0 {
		return r.allSources()
	}
	return result
}
