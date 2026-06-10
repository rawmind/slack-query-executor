package handler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

type fakeExecutor struct {
	mu        sync.Mutex
	callCount int
	lastEntry store.PendingEntry
	lastUser  string
	returnErr error
}

func (f *fakeExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	f.lastEntry = entry
	f.lastUser = approverID
	return f.returnErr
}

func newTestHandler(api slackAPIClient, exec *fakeExecutor) *Handler {
	h := &Handler{
		api:             api,
		channelID:       "C_CHANNEL",
		botUserID:       "U_BOT",
		approverGroupID: "S_GROUP",
		approveEmoji:    "white_check_mark",
		store:           store.New(),
		exec:            exec,
		seenTS:          make(map[string]time.Time),
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

func buildReactionEvent(reaction, userID, itemChannel, itemTS, itemType string) *socketmode.Event {
	inner := &slackevents.ReactionAddedEvent{
		User:     userID,
		Reaction: reaction,
		Item: slackevents.Item{
			Type:      itemType,
			Channel:   itemChannel,
			Timestamp: itemTS,
		},
		EventTimestamp: "9999999999.000000",
	}
	eventsAPI := slackevents.EventsAPIEvent{
		InnerEvent: slackevents.EventsAPIInnerEvent{
			Data: inner,
		},
	}
	return &socketmode.Event{
		Data: eventsAPI,
	}
}

const (
	testChannel = "C_CHANNEL"
	testTS      = "1234567890.000001"
	testUser    = "U_APPROVER"
)

func pendingEntry() store.PendingEntry {
	return store.PendingEntry{
		Channel:     testChannel,
		MessageTS:   testTS,
		SubmittedBy: "U_SUBMITTER",
		RawQuery:    `{"find": "users"}`,
		SubmittedAt: time.Now(),
	}
}

func TestHandleReaction_WrongEmoji(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{testUser}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	h.store.Set(testTS, pendingEntry())

	evt := buildReactionEvent("thumbsdown", testUser, testChannel, testTS, "message")
	h.handleReaction(evt, nil)

	if exec.callCount != 0 {
		t.Errorf("expected executor not called, got %d calls", exec.callCount)
	}
}

func TestHandleReaction_WrongItemType(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{testUser}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	h.store.Set(testTS, pendingEntry())

	evt := buildReactionEvent("white_check_mark", testUser, testChannel, testTS, "file")
	h.handleReaction(evt, nil)

	if exec.callCount != 0 {
		t.Errorf("expected executor not called, got %d calls", exec.callCount)
	}
}

func TestHandleReaction_WrongChannel(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{testUser}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	h.store.Set(testTS, pendingEntry())

	evt := buildReactionEvent("white_check_mark", testUser, "C_OTHER", testTS, "message")
	h.handleReaction(evt, nil)

	if exec.callCount != 0 {
		t.Errorf("expected executor not called, got %d calls", exec.callCount)
	}
}

func TestHandleReaction_GroupAPIError(t *testing.T) {
	api := &fakeSlackAPI{groupMembersErr: errors.New("slack API down")}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	h.store.Set(testTS, pendingEntry())

	evt := buildReactionEvent("white_check_mark", testUser, testChannel, testTS, "message")
	h.handleReaction(evt, nil)

	if exec.callCount != 0 {
		t.Errorf("expected executor not called, got %d calls", exec.callCount)
	}
}

func TestHandleReaction_UserNotInGroup(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{"U_OTHER_APPROVER"}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	h.store.Set(testTS, pendingEntry())

	evt := buildReactionEvent("white_check_mark", testUser, testChannel, testTS, "message")
	h.handleReaction(evt, nil)

	if exec.callCount != 0 {
		t.Errorf("expected executor not called, got %d calls", exec.callCount)
	}
}

func TestHandleReaction_TSNotInStore(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{testUser}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	evt := buildReactionEvent("white_check_mark", testUser, testChannel, testTS, "message")
	h.handleReaction(evt, nil)

	if exec.callCount != 0 {
		t.Errorf("expected executor not called, got %d calls", exec.callCount)
	}
}

func TestHandleReaction_HappyPath(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{testUser}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	entry := pendingEntry()
	h.store.Set(testTS, entry)

	evt := buildReactionEvent("white_check_mark", testUser, testChannel, testTS, "message")
	h.handleReaction(evt, nil)

	exec.mu.Lock()
	defer exec.mu.Unlock()
	if exec.callCount != 1 {
		t.Errorf("expected executor called once, got %d calls", exec.callCount)
	}
	if exec.lastUser != testUser {
		t.Errorf("expected approverID %s, got %s", testUser, exec.lastUser)
	}
	if exec.lastEntry.MessageTS != entry.MessageTS {
		t.Errorf("expected entry ts %s, got %s", entry.MessageTS, exec.lastEntry.MessageTS)
	}
}

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
	if _, ok := h.store.Delete(testTS); !ok {
		t.Error("expected pending entry in store after handleMessage")
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

func TestHandleReaction_ConcurrentSameEvent(t *testing.T) {
	api := &fakeSlackAPI{groupMembers: []string{testUser}}
	exec := &fakeExecutor{}
	h := newTestHandler(api, exec)
	h.store.Set(testTS, pendingEntry())

	evt := buildReactionEvent("white_check_mark", testUser, testChannel, testTS, "message")

	var wg sync.WaitGroup
	const goroutines = 10
	var ready int64
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()

			atomic.AddInt64(&ready, 1)
			for atomic.LoadInt64(&ready) < goroutines {

			}
			h.handleReaction(evt, nil)
		}()
	}
	wg.Wait()

	exec.mu.Lock()
	defer exec.mu.Unlock()
	if exec.callCount != 1 {
		t.Errorf("expected executor called exactly once, got %d calls", exec.callCount)
	}
}
