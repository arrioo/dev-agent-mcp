package agent

// Store defines the interface for data persistence.
// This allows swapping between temporary (in-memory) storage and a real database.
type Store interface {
	SaveEvent(event Event) error
	GetEvents() ([]Event, error)
	SetStatus(status string) error
	GetStatus() (string, error)
}
