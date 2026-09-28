package storage

import (
	"sync"

	"dev-agent-mcp/internal/agent"
)

// MemoryStore implements agent.Store using in-memory data structures.
// This is suitable as a temporary store before moving to a DB.
type MemoryStore struct {
	mu     sync.RWMutex
	events []agent.Event
	status string
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events: make([]agent.Event, 0),
		status: "offline",
	}
}

func (m *MemoryStore) SaveEvent(event agent.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *MemoryStore) GetEvents() ([]agent.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	eventsCopy := make([]agent.Event, len(m.events))
	copy(eventsCopy, m.events)
	return eventsCopy, nil
}

func (m *MemoryStore) SetStatus(status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = status
	return nil
}

func (m *MemoryStore) GetStatus() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, nil
}
