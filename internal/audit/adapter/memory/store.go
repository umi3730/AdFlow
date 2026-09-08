package memory

import (
	"context"
	"sync"

	"github.com/umi3730/adflow/internal/audit/domain"
)

type Store struct {
	mu      sync.RWMutex
	entries []domain.Entry
}

func NewStore() *Store { return &Store{} }

func (s *Store) Append(_ context.Context, entry domain.Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

func (s *Store) List(_ context.Context, filter domain.Filter) ([]domain.Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if filter.Offset >= len(s.entries) {
		return []domain.Entry{}, nil
	}
	end := len(s.entries) - filter.Offset
	start := end - filter.Limit
	if start < 0 {
		start = 0
	}
	result := make([]domain.Entry, 0, end-start)
	for index := end - 1; index >= start; index-- {
		result = append(result, s.entries[index])
	}
	return result, nil
}
