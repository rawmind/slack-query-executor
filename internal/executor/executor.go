package executor

import (
	"context"

	"github.com/rawmind/slack-query-executor/internal/store"
)

type Executor interface {
	Execute(ctx context.Context, entry store.PendingEntry, approverID string) (*ResultFile, error)
}
