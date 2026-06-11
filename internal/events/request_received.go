package events

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/rawmind/slack-query-executor/internal/deps"
	"github.com/rawmind/slack-query-executor/internal/parser"
	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
)

type RequestReceivedEvent struct {
	deps deps.Deps
}

func NewRequestReceivedEvent(deps deps.Deps) *RequestReceivedEvent {
	return &RequestReceivedEvent{
		deps:   deps,
	}
}

func (h *RequestReceivedEvent) Process(text string, channel string, user string, ts string) {
	logger := slog.With(
		"event", "request_received",
		"channel", channel,
		"user", user,
		"ts", ts,
	)
	content, ok := parser.ExtractCodeBlock(text)
	if !ok {
		h.postReply(channel, ts,
			":x: No query found. Wrap your MongoDB query in triple backticks.")
		logger.Info("message dropped: no code block")
		return
	}

	if content == "" {
		h.postReply(channel, ts,
			":x: Code block is empty. Provide a MongoDB query document inside the backticks.")
		logger.Info("message dropped: empty code block")
		return
	}
	pendingStore := h.deps.Store()
	stored := pendingStore.SetIfAbsent(ts, store.PendingEntry{
		Channel:     channel,
		MessageTS:   ts,
		SubmittedBy: user,
		RawQuery:    content,
		SubmittedAt: time.Now(),
	})
	if !stored {
		logger.Info("message ignored: already seen")
		return
	}
	approveEmoji := h.deps.Config().ApproveEmoji
	h.postReply(channel, ts,
		fmt.Sprintf(":white_check_mark: Query received and stored. React with :%s: to execute.", approveEmoji))

	logger.Info("query stored")
}

func (e *RequestReceivedEvent) postReply(channel, threadTS, text string) {
	_, _, err := e.deps.SlackAPI().PostMessage(
		channel,
		slack.MsgOptionTS(threadTS),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		slog.Error("PostMessage failed",
			"channel", channel, "thread_ts", threadTS, "err", err)
	}
}