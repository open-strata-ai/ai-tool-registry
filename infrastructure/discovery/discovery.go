// Package discovery is an in-memory domain.Discovery (DESIGN §5.3 / §6.5).
// Production backs this with K8s Endpoints + Redis instance lists.
package discovery

import (
	"sync"

	"github.com/open-strata-ai/ai-tool-registry/domain"
)

// InMemory stores tool instances per ToolID.
type InMemory struct {
	mu        sync.RWMutex
	instances map[domain.ToolID][]domain.ToolInstance
}

// New builds an empty discovery.
func New() *InMemory {
	return &InMemory{instances: map[domain.ToolID][]domain.ToolInstance{}}
}

// RegisterInstance adds or replaces an instance for toolID (matched by endpoint).
func (m *InMemory) RegisterInstance(id domain.ToolID, inst domain.ToolInstance) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.instances[id]
	for i, e := range list {
		if e.Endpoint == inst.Endpoint {
			list[i] = inst
			return
		}
	}
	m.instances[id] = append(list, inst)
}

// DeregisterInstance removes the instance with the given endpoint.
func (m *InMemory) DeregisterInstance(id domain.ToolID, endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.instances[id]
	out := make([]domain.ToolInstance, 0, len(list))
	for _, e := range list {
		if e.Endpoint != endpoint {
			out = append(out, e)
		}
	}
	m.instances[id] = out
}

// Instances returns a copy of the registered instances for toolID.
func (m *InMemory) Instances(id domain.ToolID) []domain.ToolInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.instances[id]
	out := make([]domain.ToolInstance, len(list))
	copy(out, list)
	return out
}

var _ domain.Discovery = (*InMemory)(nil)
