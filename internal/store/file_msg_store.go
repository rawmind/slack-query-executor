package store

import (
	"sync"
	"time"
)

type FileMsgStore struct {
	mu      sync.Mutex
	entries map[string]fileMsgEntry
	ttl     time.Duration
}

type fileMsgEntry struct {
	msgTS     string
	expiresAt time.Time
}

// Keep entries alive slightly beyond the TTL so the AfterFunc always finds them.
func NewFileMsgStore(ttl time.Duration) *FileMsgStore {
	return &FileMsgStore{entries: make(map[string]fileMsgEntry), ttl: ttl}
}

func (s *FileMsgStore) Set(fileID, msgTS string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	s.entries[fileID] = fileMsgEntry{msgTS: msgTS, expiresAt: time.Now().Add(s.ttl)}
}

func (s *FileMsgStore) Get(fileID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[fileID]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.msgTS, true
}

func (s *FileMsgStore) sweep() {
	now := time.Now()
	for k, v := range s.entries {
		if now.After(v.expiresAt) {
			delete(s.entries, k)
		}
	}
}
