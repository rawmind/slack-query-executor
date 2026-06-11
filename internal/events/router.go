package events

import (
	"log/slog"
	"sync"
	"time"

	"github.com/rawmind/slack-query-executor/internal/deps"
	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type slackAPIClient = deps.SlackAPIClient

const seenTTL = 24 * time.Hour

type Router struct {
	channelID       string
	botUserID       string
	fileMsgStore    *store.FileMsgStore
	smHandler       *socketmode.SocketmodeHandler
	seenMu          sync.Mutex
	seenTS          map[string]time.Time
	deps            deps.Deps
}

func NewRouter(deps deps.Deps, client *socketmode.Client, botUserID string) *Router {
	h := &Router{
		channelID:       deps.Config().ChannelID,
		botUserID:       botUserID,
		fileMsgStore:    deps.FileMsgStore(),
		seenTS:          make(map[string]time.Time),
		deps:            deps,
	}
	h.smHandler = socketmode.NewSocketmodeHandler(client)
	return h
}

func (h *Router) Register() {
	h.smHandler.HandleEvents(slackevents.Message, h.handleMessage)
	h.smHandler.HandleEvents(slackevents.AppMention, h.handleMessage)
	h.smHandler.HandleEvents(slackevents.ReactionAdded, h.handleReaction)
	h.smHandler.HandleDefault(h.logUnhandled)
}

func (h *Router) Run() error {
	return h.smHandler.RunEventLoop()
}

func (h *Router) handleMessage(evt *socketmode.Event, client *socketmode.Client) {
	eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	var channel, user, subType, ts, text string
	var msgEv *slackevents.MessageEvent
	switch ev := eventsAPI.InnerEvent.Data.(type) {
	case *slackevents.MessageEvent:
		msgEv = ev
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

	if h.fileMsgStore != nil && subType == slack.MsgSubTypeFileShare && user == h.botUserID {
		if msgEv != nil && msgEv.Message != nil {
			for _, f := range msgEv.Message.Files {
				h.fileMsgStore.Set(f.ID, ts)
				slog.Info("file message TS captured", "file_id", f.ID, "ts", ts)
			}
		}
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
	NewRequestReceivedEvent(h.deps).Process(text, channel, user, ts)
}

func (h *Router) handleReaction(evt *socketmode.Event, client *socketmode.Client) {
	NewApprovedEvent(h.deps).Process(evt)
}

func (h *Router) firstSeen(ts string) bool {
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

func (h *Router) logUnhandled(evt *socketmode.Event, client *socketmode.Client) {
	slog.Info("unhandled event", "type", string(evt.Type))
}
