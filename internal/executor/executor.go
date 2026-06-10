package executor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
)

type Executor interface {
	Execute(ctx context.Context, entry store.PendingEntry, approverID string) error
}

type LogExecutor struct {
	api *slack.Client
}

func NewLogExecutor(api *slack.Client) *LogExecutor {
	return &LogExecutor{api: api}
}

func (e *LogExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) error {
	slog.Info("query approved",
		"ts", entry.MessageTS,
		"channel", entry.Channel,
		"approver", approverID,
		"query", entry.RawQuery,
	)

	text := fmt.Sprintf(":white_check_mark: Approved by <@%s>. Execution not yet implemented.", approverID)
	_, _, err := e.api.PostMessageContext(
		ctx,
		entry.Channel,
		slack.MsgOptionTS(entry.MessageTS),
		slack.MsgOptionText(text, false),
	)
	return err
}
