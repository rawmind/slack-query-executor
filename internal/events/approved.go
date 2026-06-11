package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rawmind/slack-query-executor/internal/config"
	"github.com/rawmind/slack-query-executor/internal/deps"
	"github.com/rawmind/slack-query-executor/internal/executor"
	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type ApprovedEvent struct {
	deps deps.Deps
}

func NewApprovedEvent(deps deps.Deps) *ApprovedEvent {
	return &ApprovedEvent{deps: deps}
}

func (e *ApprovedEvent) Process(evt *socketmode.Event) {
	cfg := e.deps.Config()
	if cfg == nil {
		return
	}

	api := e.deps.SlackAPI()
	pendingStore := e.deps.Store()
	if api == nil || pendingStore == nil {
		return
	}

	eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return
	}

	ev, ok := eventsAPI.InnerEvent.Data.(*slackevents.ReactionAddedEvent)
	if !ok {
		return
	}

	expectedChannel := cfg.ChannelID
	channel := ev.Item.Channel
	user := ev.User
	emoji := ev.Reaction
	itemTimestamp := ev.Item.Timestamp
	itemType := ev.Item.Type

	approvalMode := strings.ToLower(cfg.ApprovalMode)

	logger := slog.With(
		"event", "reaction_added",
		"channel", channel,
		"approval_mode", approvalMode,
		"approved_user_ids", cfg.ApprovedUserIds,
		"emoji", emoji,
		"user", user,
		"item_timestamp", itemTimestamp,
	)

	logger.Info("reaction event received")
	if ev.Reaction != cfg.ApproveEmoji {
		logger.Info("reaction ignored: wrong emoji", "expected", cfg.ApproveEmoji)
		return
	}

	if itemType != "message" {
		logger.Warn("reaction ignored: unsupported item type",
			"item_type", itemType, "user", user)
		return
	}
	if channel != expectedChannel {
		logger.Info("reaction ignored: non-target channel")
		return
	}

	ctx := context.Background()
	members, err := approvalModeGroupMembers(ctx, api, cfg)
	if err != nil {
		logger.Error("GetUserGroupMembersContext failed", "err", err)
		return
	}
	logger.Debug("approver group members fetched", "member_count", len(members))
	found := false
	for _, m := range members {
		if m == ev.User {
			found = true
			break
		}
	}
	if !found {
		logger.Debug("reaction from non-approver ignored")
		return
	}

	entry, ok := pendingStore.Delete(ev.Item.Timestamp)
	if !ok {
		logger.Debug("reaction ignored: no pending entry")
		return
	}
	logger.Debug("approval accepted and pending entry removed", "ts", entry.MessageTS)
	rf, err := e.deps.Executor().Execute(ctx, entry, ev.User)
	if err != nil {
		logger.Error("executor failed", "ts", entry.MessageTS, "err", err)
		e.postError(ctx, entry, ":x: Query execution error: "+err.Error())
		return
	}

	jsonBytes, _ := json.MarshalIndent(rf, "", "  ")
	jsonBytes = executor.ReplaceOIDTokens(jsonBytes)
	ts := strings.ReplaceAll(entry.MessageTS, ".", "-")
	filename := fmt.Sprintf("query-results-%s.json", ts)
	initialComment := ""
	if rf.Truncated {
		initialComment = fmt.Sprintf("Results truncated to %d documents (soft cap).", rf.MaxDocCap)
	}

	uploadedFile, err := e.deps.SlackAPI().UploadFileContext(ctx, slack.UploadFileParameters{
		Channel:         entry.Channel,
		ThreadTimestamp: entry.MessageTS,
		Filename:        filename,
		Title:           filename,
		Content:         string(jsonBytes),
		FileSize:        len(jsonBytes),
		InitialComment:  initialComment,
	})

	e.scheduleDelete(uploadedFile.ID, entry.Channel)
}

func approvalModeGroupMembers(ctx context.Context, api slackAPIClient, cfg *config.Config) ([]string, error) {
	members := []string{}
	if cfg.ApprovalMode == "group" {
		if cfg.ApproverGroupID == "" {
			return nil, fmt.Errorf("approver group ID is not configured")
		}
		members, err := api.GetUserGroupMembersContext(ctx, cfg.ApproverGroupID)
		if err != nil {
			return nil, fmt.Errorf("GetUserGroupMembersContext failed: %w", err)
		}
		return members, nil
	}
	approvedUserIds := parseApprovedUserIds(cfg.ApprovedUserIds)
	if len(approvedUserIds) > 0 {
		members = append(members, approvedUserIds...)
	}
	return members, nil
}

func parseApprovedUserIds(s string) []string {
	var ids []string
	for _, id := range strings.Split(s, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (e *ApprovedEvent) scheduleDelete(fileID, channel string) {
	messageTTL := e.deps.Config().MessageTTL
	if messageTTL <= 0 {
		return
	}
	slog.Debug("TTL scheduled", "file_id", fileID, "ttl", messageTTL)
	time.AfterFunc(messageTTL, func() {
		ctx := context.Background()
		slackAPI := e.deps.SlackAPI()
		var msgTS string
		if e.deps.FileMsgStore() != nil {
			msgTS, _ = e.deps.FileMsgStore().Get(fileID)
		}

		if err := slackAPI.DeleteFileContext(ctx, fileID); err != nil {
			slog.Error("TTL: failed to delete file", "file_id", fileID, "err", err)
		} else {
			slog.Info("TTL: deleted file", "file_id", fileID)
		}

		if msgTS != "" {
			if _, _, err := slackAPI.DeleteMessage(channel, msgTS); err != nil {
				slog.Error("TTL: failed to delete message", "channel", channel, "ts", msgTS, "err", err)
			} else {
				slog.Info("TTL: deleted message", "channel", channel, "ts", msgTS)
			}
		}
	})
}

func (e *ApprovedEvent) postError(ctx context.Context, entry store.PendingEntry, text string) {
	_, _, err := e.deps.SlackAPI().PostMessageContext(ctx, entry.Channel,
		slack.MsgOptionTS(entry.MessageTS),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		slog.Error("PostMessageContext failed", "err", err)
	}
}

