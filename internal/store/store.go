package store

import (
	"sync"
	"time"
)

const executedTTL = 24 * time.Hour

type PendingEntry struct {
	Channel     string
	MessageTS   string
	SubmittedBy string
	RawQuery    string
	SubmittedAt time.Time
}

type PendingStore struct {
	mu       sync.RWMutex
	entries  map[string]PendingEntry
	executed map[string]time.Time
}

func New() *PendingStore {
	return &PendingStore{
		entries:  make(map[string]PendingEntry),
		executed: make(map[string]time.Time),
	}
}

func (s *PendingStore) Set(ts string, e PendingEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[ts] = e
}

func (s *PendingStore) SetIfAbsent(ts string, e PendingEntry) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entries[ts]; exists {
		return false
	}
	if _, executed := s.executed[ts]; executed {
		return false
	}

	s.entries[ts] = e
	return true
}

func (s *PendingStore) Get(ts string) (PendingEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[ts]
	return e, ok
}

func (s *PendingStore) Delete(ts string) (PendingEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[ts]
	if ok {
		delete(s.entries, ts)
		s.executed[ts] = time.Now()
		s.evictExpiredLocked()
	}
	return e, ok
}

func (s *PendingStore) evictExpiredLocked() {
	cutoff := time.Now().Add(-executedTTL)
	for ts, t := range s.executed {
		if t.Before(cutoff) {
			delete(s.executed, ts)
		}
	}
}
