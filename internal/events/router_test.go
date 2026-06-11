package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rawmind/slack-query-executor/internal/config"
	"github.com/rawmind/slack-query-executor/internal/deps"
	"github.com/rawmind/slack-query-executor/internal/executor"
	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type fakeSlackAPI struct {
	mu              sync.Mutex
	postMessages    []string
	groupMembers    []string
	groupMembersErr error
}

func (f *fakeSlackAPI) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.postMessages = append(f.postMessages, channelID)
	return "", "", nil
}

func (f *fakeSlackAPI) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.postMessages = append(f.postMessages, channelID)
	return "", "", nil
}

func (f *fakeSlackAPI) GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slack.GetUserGroupMembersOption) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.groupMembers, f.groupMembersErr
}

func (f *fakeSlackAPI) DeleteFileContext(ctx context.Context, fileID string) error {
	return nil
}

func (f *fakeSlackAPI) DeleteMessage(channelID, timestamp string) (string, string, error) {
	return "", "", nil
}

func (f *fakeSlackAPI) UploadFileContext(ctx context.Context, params slack.UploadFileParameters) (*slack.FileSummary, error) {
	return &slack.FileSummary{}, nil
}

type fakeExecutor struct {
	mu        sync.Mutex
	callCount int
	lastEntry store.PendingEntry
	lastUser  string
	returnErr error
}

func (f *fakeExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) (*executor.ResultFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	f.lastEntry = entry
	f.lastUser = approverID
	return nil, f.returnErr
}

func newTestHandler(api slackAPIClient, exec *fakeExecutor) *Router {
	pending := store.New()
	cfg := &config.Config{
		ChannelID:       "C_CHANNEL",
		ApproverGroupID: "S_GROUP",
		ApproveEmoji:    "white_check_mark",
	}
	testDeps := deps.NewDeps(api, pending, nil, cfg, exec)

	h := &Router{
		channelID:       "C_CHANNEL",
		botUserID:       "U_BOT",
		seenTS:          make(map[string]time.Time),
		deps:            testDeps,
	}
	return h
}

func buildMessageEvent(channel, userID, ts, text, subType string) *socketmode.Event {
	inner := &slackevents.MessageEvent{
		Channel:   channel,
		User:      userID,
		TimeStamp: ts,
		Text:      text,
		SubType:   subType,
	}
	eventsAPI := slackevents.EventsAPIEvent{
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "message",
			Data: inner,
		},
	}
	return &socketmode.Event{Data: eventsAPI}
}

func buildAppMentionEvent(channel, userID, ts, text string) *socketmode.Event {
	inner := &slackevents.AppMentionEvent{
		Channel:   channel,
		User:      userID,
		TimeStamp: ts,
		Text:      text,
	}
	eventsAPI := slackevents.EventsAPIEvent{
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Type: "app_mention",
			Data: inner,
		},
	}
	return &socketmode.Event{Data: eventsAPI}
}

const (
	testChannel = "C_CHANNEL"
	testTS      = "1234567890.000001"
	testUser    = "U_APPROVER"
)

func TestHandleMessage_StoresQueryAndAcksOnce(t *testing.T) {
	api := &fakeSlackAPI{}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)

	text := "```db.users.find({})```"
	evt := buildMessageEvent(testChannel, testUser, testTS, text, "")
	h.handleMessage(evt, nil)

	api.mu.Lock()
	msgCount := len(api.postMessages)
	api.mu.Unlock()
	if msgCount != 1 {
		t.Errorf("expected 1 acknowledgment post, got %d", msgCount)
	}
}

func TestHandleMessage_AppMentionEventHandled(t *testing.T) {
	api := &fakeSlackAPI{}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)

	text := "```db.users.find({})```"
	evt := buildAppMentionEvent(testChannel, testUser, testTS, text)
	h.handleMessage(evt, nil)

	api.mu.Lock()
	msgCount := len(api.postMessages)
	api.mu.Unlock()
	if msgCount != 1 {
		t.Errorf("expected 1 ack for app_mention inner event, got %d", msgCount)
	}
}

func TestHandleMessage_NoDoubleAckOnDuplicateEvent(t *testing.T) {
	api := &fakeSlackAPI{}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)

	text := "```db.users.find({})```"

	msgEvt := buildMessageEvent(testChannel, testUser, testTS, text, "")
	mentionEvt := buildAppMentionEvent(testChannel, testUser, testTS, text)
	h.handleMessage(msgEvt, nil)
	h.handleMessage(mentionEvt, nil)

	api.mu.Lock()
	msgCount := len(api.postMessages)
	api.mu.Unlock()
	if msgCount != 1 {
		t.Errorf("expected exactly 1 ack for duplicate events, got %d", msgCount)
	}
}

func TestHandleMessage_BotSelfMessageIgnored(t *testing.T) {
	api := &fakeSlackAPI{}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)

	evt := buildMessageEvent(testChannel, "U_BOT", testTS, "```db.users.find({})```", "")
	h.handleMessage(evt, nil)

	api.mu.Lock()
	msgCount := len(api.postMessages)
	api.mu.Unlock()
	if msgCount != 0 {
		t.Errorf("expected 0 posts for bot self-message, got %d", msgCount)
	}
}

func TestHandleMessage_WrongChannelIgnored(t *testing.T) {
	api := &fakeSlackAPI{}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)

	evt := buildMessageEvent("C_OTHER", testUser, testTS, "```db.users.find({})```", "")
	h.handleMessage(evt, nil)

	api.mu.Lock()
	msgCount := len(api.postMessages)
	api.mu.Unlock()
	if msgCount != 0 {
		t.Errorf("expected 0 posts for wrong channel, got %d", msgCount)
	}
}

func TestHandleMessage_NoDoubleErrorReplyOnMissingCodeBlock(t *testing.T) {
	api := &fakeSlackAPI{}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)

	text := "please run this query for me"
	msgEvt := buildMessageEvent(testChannel, testUser, testTS, text, "")
	mentionEvt := buildAppMentionEvent(testChannel, testUser, testTS, text)

	h.handleMessage(msgEvt, nil)
	h.handleMessage(mentionEvt, nil)

	api.mu.Lock()
	msgCount := len(api.postMessages)
	api.mu.Unlock()
	if msgCount != 1 {
		t.Errorf("expected exactly 1 error reply for duplicate events with no code block, got %d", msgCount)
	}
}
