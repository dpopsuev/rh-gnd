package dsr

import (
	"context"
	"fmt"
	"io/fs"
	"sync"

	"github.com/dpopsuev/origami/toolkit"
)

// RouterOption configures an AccessRouter.
type RouterOption func(*AccessRouter)

// WithGitDriver registers a git driver.
func WithGitDriver(d Driver) RouterOption {
	return func(r *AccessRouter) { r.Register(d) }
}

// WithDocsDriver registers a documentation driver.
func WithDocsDriver(d Driver) RouterOption {
	return func(r *AccessRouter) { r.Register(d) }
}

// WithOfflineFS registers OfflineFSDrivers for both repo and doc source kinds,
// backed by the given filesystem. Use this instead of WithGitDriver/WithDocsDriver
// when running in offline calibration mode.
func WithOfflineFS(fsys fs.FS) RouterOption {
	return func(r *AccessRouter) {
		r.Register(NewOfflineFSDriver(fsys, toolkit.SourceKindRepo))
		r.Register(NewOfflineFSDriver(fsys, toolkit.SourceKindDoc))
	}
}

// AccessRouter dispatches Reader operations to the appropriate Driver
// based on the source's Kind. It implements the Reader interface.
type AccessRouter struct {
	mu      sync.RWMutex
	drivers map[toolkit.SourceKind]Driver
}

// NewRouter creates an AccessRouter with the given options.
func NewRouter(opts ...RouterOption) *AccessRouter {
	r := &AccessRouter{
		drivers: make(map[toolkit.SourceKind]Driver),
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds a driver to the router. If a driver for the same kind
// already exists, it is replaced.
func (r *AccessRouter) Register(d Driver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.drivers[d.Handles()] = d
}

func (r *AccessRouter) driver(kind toolkit.SourceKind) (Driver, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[kind]
	if !ok {
		return nil, fmt.Errorf("no driver registered for source kind %q", kind)
	}
	return d, nil
}

func (r *AccessRouter) Ensure(ctx context.Context, src toolkit.Source) error {
	d, err := r.driver(src.Kind)
	if err != nil {
		return err
	}
	return d.Ensure(ctx, src)
}

func (r *AccessRouter) Search(ctx context.Context, src toolkit.Source, query string, maxResults int) ([]toolkit.SearchResult, error) {
	d, err := r.driver(src.Kind)
	if err != nil {
		return nil, err
	}
	return d.Search(ctx, src, query, maxResults)
}

func (r *AccessRouter) Read(ctx context.Context, src toolkit.Source, path string) ([]byte, error) {
	d, err := r.driver(src.Kind)
	if err != nil {
		return nil, err
	}
	return d.Read(ctx, src, path)
}

func (r *AccessRouter) List(ctx context.Context, src toolkit.Source, root string, maxDepth int) ([]toolkit.ContentEntry, error) {
	d, err := r.driver(src.Kind)
	if err != nil {
		return nil, err
	}
	return d.List(ctx, src, root, maxDepth)
}

// Ready reports whether at least one driver is registered.
func (r *AccessRouter) Ready() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.drivers) > 0
}

// Compile-time check that AccessRouter implements Reader.
var _ toolkit.SourceReader = (*AccessRouter)(nil)
