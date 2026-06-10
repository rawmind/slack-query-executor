package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rawmind/slack-query-executor/internal/store"
)

type fakeSlackClient struct {
	postErr     error
	callCount   int
	lastChannel string
	lastText    string
}

func (f *fakeSlackClient) PostMessageContext(ctx context.Context, channelID string, options ...interface{}) (string, string, error) {
	f.callCount++
	f.lastChannel = channelID
	return "", "", f.postErr
}

type postFunc func(ctx context.Context, channel string) error

type testExecutor struct {
	post postFunc
}

func (t *testExecutor) execute(ctx context.Context, entry store.PendingEntry, approverID string) error {
	slog := func() {}
	_ = slog
	return t.post(ctx, entry.Channel)
}

func TestLogExecutorErrorPropagation(t *testing.T) {
	entry := store.PendingEntry{
		Channel:     "C123",
		MessageTS:   "1234567890.000001",
		SubmittedBy: "U_SUBMITTER",
		RawQuery:    `{"find": "users"}`,
		SubmittedAt: time.Now(),
	}

	tests := []struct {
		name       string
		postErr    error
		wantErrNil bool
	}{
		{
			name:       "PostMessage success returns nil",
			postErr:    nil,
			wantErrNil: true,
		},
		{
			name:       "PostMessage error is returned by Execute",
			postErr:    errors.New("slack API unavailable"),
			wantErrNil: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			exec := &callbackExecutor{
				fn: func(ctx context.Context, e store.PendingEntry, approverID string) error {
					called = true
					return tc.postErr
				},
			}

			err := exec.fn(context.Background(), entry, "U_APPROVER")
			if !called {
				t.Fatal("expected execute to be called")
			}
			if tc.wantErrNil && err != nil {
				t.Errorf("expected nil error, got %v", err)
			}
			if !tc.wantErrNil && err == nil {
				t.Error("expected non-nil error, got nil")
			}
		})
	}
}

type callbackExecutor struct {
	fn func(ctx context.Context, entry store.PendingEntry, approverID string) error
}

func (c *callbackExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) error {
	return c.fn(ctx, entry, approverID)
}

func TestNewLogExecutor(t *testing.T) {
	exec := NewLogExecutor(nil)
	if exec == nil {
		t.Fatal("NewLogExecutor returned nil")
	}
}
