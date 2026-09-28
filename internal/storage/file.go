package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"dev-agent-mcp/internal/agent"
)

// FileStore implements agent.Store using a local file.
// It saves data to a temporary JSON file within the project directory.
type FileStore struct {
	mu       sync.RWMutex
	filePath string
	data     struct {
		Events []agent.Event `json:"events"`
		Status string        `json:"status"`
	}
}

func NewFileStore(path string) (*FileStore, error) {
	// Create the directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	fs := &FileStore{
		filePath: path,
	}
	fs.data.Events = make([]agent.Event, 0)
	fs.data.Status = "offline"

	// Load existing data if the file exists
	b, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(b, &fs.data)
	}

	return fs, nil
}

func (s *FileStore) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, b, 0644)
}

func (s *FileStore) SaveEvent(event agent.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Events = append(s.data.Events, event)
	return s.save()
}

func (s *FileStore) GetEvents() ([]agent.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	eventsCopy := make([]agent.Event, len(s.data.Events))
	copy(eventsCopy, s.data.Events)
	return eventsCopy, nil
}

func (s *FileStore) SetStatus(status string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Status = status
	return s.save()
}

func (s *FileStore) GetStatus() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Status, nil
}
