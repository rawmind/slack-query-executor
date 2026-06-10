package handler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"github.com/rawmind/slack-query-executor/internal/executor"
	"github.com/rawmind/slack-query-executor/internal/parser"
	"github.com/rawmind/slack-query-executor/internal/store"
)

type slackAPIClient interface {
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slack.GetUserGroupMembersOption) ([]string, error)
}

const seenTTL = 24 * time.Hour

type Handler struct {
	api             slackAPIClient
	channelID       string
	botUserID       string
	approverGroupID string
	approveEmoji    string
	store           *store.PendingStore
	smHandler       *socketmode.SocketmodeHandler
	exec            executor.Executor
	seenMu          sync.Mutex
	seenTS          map[string]time.Time
}

func New(api *slack.Client, client *socketmode.Client, channelID string, botUserID string, approverGroupID string, approveEmoji string, exec executor.Executor) *Handler {
	h := &Handler{
		api:             api,
		channelID:       channelID,
		botUserID:       botUserID,
		approverGroupID: approverGroupID,
		approveEmoji:    approveEmoji,
		store:           store.New(),
		exec:            exec,
		seenTS:          make(map[string]time.Time),
	}
	h.smHandler = socketmode.NewSocketmodeHandler(client)
	return h
}

func (h *Handler) Register() {
	h.smHandler.HandleEvents(slackevents.Message, h.handleMessage)
	h.smHandler.HandleEvents(slackevents.AppMention, h.handleMessage)
	h.smHandler.HandleEvents(slackevents.ReactionAdded, h.handleReaction)
	h.smHandler.HandleDefault(h.logUnhandled)
}

func (h *Handler) Run() error {
	return h.smHandler.RunEventLoop()
}

func (h *Handler) handleMessage(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	var channel, user, subType, ts, text string
	switch ev := eventsAPI.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		channel = ev.Channel
		user = ev.User
		subType = ev.SubType
		ts = ev.TimeStamp
		text = ev.Text
		slog.Info("message event received",
			"channel", channel, "user", user, "subtype", subType, "ts", ts)
	case *slackevents.AppMentionEvent:
		channel = ev.Channel
		user = ev.User
		ts = ev.TimeStamp
		text = ev.Text
		slog.Info("app_mention event received",
			"channel", channel, "user", user, "ts", ts)
	default:
		return
	}

	if channel != h.channelID {
		slog.Info("message ignored: non-target channel",
			"channel", channel, "expected", h.channelID, "user", user, "ts", ts)
		return
	}

	if user == h.botUserID {
		slog.Info("message ignored: bot self-message", "channel", channel, "ts", ts)
		return
	}

	if subType != "" {
		slog.Info("message ignored: unsupported subtype",
			"subtype", subType, "channel", channel, "user", user, "ts", ts)
		return
	}

	if !h.firstSeen(ts) {
		slog.Info("message ignored: duplicate event for ts", "ts", ts)
		return
	}

	content, ok := parser.ExtractCodeBlock(text)
	if !ok {
		h.postReply(channel, ts,
			":x: No query found. Wrap your MongoDB query in triple backticks.")
		slog.Info("message dropped: no code block",
			"channel", channel, "user", user, "ts", ts)
		return
	}

	if content == "" {
		h.postReply(channel, ts,
			":x: Code block is empty. Provide a MongoDB query document inside the backticks.")
		return
	}

	stored := h.store.SetIfAbsent(ts, store.PendingEntry{
		Channel:     channel,
		MessageTS:   ts,
		SubmittedBy: user,
		RawQuery:    content,
		SubmittedAt: time.Now(),
	})
	if !stored {
		slog.Info("message ignored: duplicate event for ts", "ts", ts)
		return
	}

	h.postReply(channel, ts,
		fmt.Sprintf(":white_check_mark: Query received and stored. React with :%s: to execute.", h.approveEmoji))

	slog.Info("query stored",
		"ts", ts, "user", user, "channel", channel)
}

func (h *Handler) handleReaction(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	ev, ok := eventsAPI.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
	if !ok {
		return
	}

	slog.Info("reaction event received",
		"channel", ev.Item.Channel, "user", ev.User, "emoji", ev.Reaction, "item_ts", ev.Item.Timestamp)

	if ev.User == h.botUserID {
		slog.Info("reaction ignored: bot self-reaction", "user", ev.User)
		return
	}

	if ev.Reaction != h.approveEmoji {
		slog.Info("reaction ignored: wrong emoji",
			"emoji", ev.Reaction, "expected", h.approveEmoji, "user", ev.User)
		return
	}

	if ev.Item.Type != "message" {
		slog.Info("reaction ignored: unsupported item type",
			"item_type", ev.Item.Type, "user", ev.User)
		return
	}

	if ev.Item.Channel != h.channelID {
		slog.Info("reaction ignored: non-target channel",
			"channel", ev.Item.Channel, "expected", h.channelID, "user", ev.User)
		return
	}

	ctx := context.Background()
	members, err := h.api.GetUserGroupMembersContext(ctx, h.approverGroupID)
	if err != nil {
		slog.Error("GetUserGroupMembersContext failed",
			"group", h.approverGroupID, "user", ev.User, "err", err)
		return
	}
	found := false
	for _, m := range members {
		if m == ev.User {
			found = true
			break
		}
	}
	if !found {
		slog.Info("reaction from non-approver ignored",
			"user", ev.User, "emoji", ev.Reaction)
		return
	}

	entry, ok := h.store.Delete(ev.Item.Timestamp)
	if !ok {
		slog.Info("reaction ignored: no pending entry",
			"message_ts", ev.Item.Timestamp, "user", ev.User)
		return
	}

	if err := h.exec.Execute(ctx, entry, ev.User); err != nil {
		slog.Error("executor failed", "ts", entry.MessageTS, "err", err)
	}
}

func (h *Handler) postReply(channel, threadTS, text string) {
	_, _, err := h.api.PostMessage(
		channel,
		slack.MsgOptionTS(threadTS),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		slog.Error("PostMessage failed",
			"channel", channel, "thread_ts", threadTS, "err", err)
	}
}

func (h *Handler) firstSeen(ts string) bool {
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	if _, exists := h.seenTS[ts]; exists {
		return false
	}
	h.seenTS[ts] = time.Now()
	cutoff := time.Now().Add(-seenTTL)
	for k, t := range h.seenTS {
		if t.Before(cutoff) {
			delete(h.seenTS, k)
		}
	}
	return true
}

func (h *Handler) logUnhandled(evt *socketmode.Event, client *socketmode.Client) {
	slog.Info("unhandled event", "type", string(evt.Type))
}
