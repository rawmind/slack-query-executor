package store

import (
	"testing"
	"time"
)

func TestPendingStore_SetGet(t *testing.T) {
	s := New()
	entry := PendingEntry{
		Channel:     "C123",
		MessageTS:   "1234567890.000001",
		SubmittedBy: "U456",
		RawQuery:    `{"find":"users"}`,
		SubmittedAt: time.Now(),
	}

	s.Set(entry.MessageTS, entry)

	got, ok := s.Get(entry.MessageTS)
	if !ok {
		t.Fatal("Get: expected entry to be found after Set")
	}
	if got.RawQuery != entry.RawQuery {
		t.Errorf("Get: RawQuery = %q, want %q", got.RawQuery, entry.RawQuery)
	}
}

func TestPendingStore_Delete_Existing(t *testing.T) {
	s := New()
	entry := PendingEntry{
		Channel:     "C123",
		MessageTS:   "1234567890.000002",
		SubmittedBy: "U456",
		RawQuery:    `{"find":"orders"}`,
		SubmittedAt: time.Now(),
	}

	s.Set(entry.MessageTS, entry)

	deleted, ok := s.Delete(entry.MessageTS)
	if !ok {
		t.Fatal("Delete: expected (entry, true) for existing key")
	}
	if deleted.RawQuery != entry.RawQuery {
		t.Errorf("Delete: returned RawQuery = %q, want %q", deleted.RawQuery, entry.RawQuery)
	}

	_, ok = s.Get(entry.MessageTS)
	if ok {
		t.Fatal("Get after Delete: expected (zero, false) but got true")
	}
}

func TestPendingStore_Delete_NonExistent(t *testing.T) {
	s := New()

	_, ok := s.Delete("nonexistent.ts")
	if ok {
		t.Fatal("Delete of non-existent key: expected false")
	}
}

func TestPendingStore_SetIfAbsent_RejectsWhilePending(t *testing.T) {
	s := New()
	ts := "1111111111.000001"
	entry := PendingEntry{MessageTS: ts, RawQuery: `{"find":"col"}`}

	if !s.SetIfAbsent(ts, entry) {
		t.Fatal("SetIfAbsent: expected true on first call")
	}
	if s.SetIfAbsent(ts, entry) {
		t.Fatal("SetIfAbsent: expected false for duplicate while pending")
	}
}

func TestPendingStore_SetIfAbsent_RejectsAfterExecution(t *testing.T) {
	s := New()
	ts := "2222222222.000001"
	entry := PendingEntry{MessageTS: ts, RawQuery: `{"find":"col"}`}

	if !s.SetIfAbsent(ts, entry) {
		t.Fatal("SetIfAbsent: expected true on first call")
	}
	if _, ok := s.Delete(ts); !ok {
		t.Fatal("Delete: expected (entry, true)")
	}

	if s.SetIfAbsent(ts, entry) {
		t.Fatal("SetIfAbsent: expected false for ts that was already executed (duplicate-result regression)")
	}
}

func TestPendingStore_evictExpiredLocked(t *testing.T) {
	s := New()
	ts := "3333333333.000001"

	s.mu.Lock()
	s.executed[ts] = time.Now().Add(-(executedTTL + time.Second))
	s.mu.Unlock()

	otherTS := "3333333333.000002"
	s.Set(otherTS, PendingEntry{MessageTS: otherTS})
	s.Delete(otherTS)

	s.mu.RLock()
	_, still := s.executed[ts]
	s.mu.RUnlock()

	if still {
		t.Fatal("evictExpiredLocked: expected expired executed entry to be removed")
	}
}
