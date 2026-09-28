package agent

import (
	"time"
)

// Monitor acts as the business logic layer, using a Store interface for data.
type Monitor struct {
	store Store
}

func NewMonitor(store Store) *Monitor {
	return &Monitor{
		store: store,
	}
}

func (m *Monitor) GetStatus() string {
	status, _ := m.store.GetStatus()
	return status
}

func (m *Monitor) SetStatus(status string) {
	m.store.SetStatus(status)
}

func (m *Monitor) RecordEvent(eventType string, payload map[string]interface{}) {
	event := Event{
		ID:        time.Now().Format("20060102150405.000"),
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}
	_ = m.store.SaveEvent(event)
}

func (m *Monitor) GetEvents() []Event {
	events, _ := m.store.GetEvents()
	return events
}
