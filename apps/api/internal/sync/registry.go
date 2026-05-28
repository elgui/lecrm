package sync

import (
	"fmt"
	"sync"
)

// Registry holds all registered providers. Providers register themselves
// at init time; the sync engine looks them up by ProviderID at runtime.
type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderID]Provider
}

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[ProviderID]Provider),
	}
}

// Register adds a provider to the registry. Panics on duplicate ID
// (a programming error, not a runtime condition).
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := p.ID()
	if _, exists := r.providers[id]; exists {
		panic(fmt.Sprintf("sync: duplicate provider registration for %q", id))
	}
	r.providers[id] = p
}

// Get returns the provider for the given ID, or an error if not registered.
func (r *Registry) Get(id ProviderID) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("sync: unknown provider %q", id)
	}
	return p, nil
}

// List returns all registered provider IDs.
func (r *Registry) List() []ProviderID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]ProviderID, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	return ids
}
