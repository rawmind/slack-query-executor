package events_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rawmind/slack-query-executor/internal/config"
	depspkg "github.com/rawmind/slack-query-executor/internal/deps"
	"github.com/rawmind/slack-query-executor/internal/events"
	"github.com/rawmind/slack-query-executor/internal/executor"
	"github.com/rawmind/slack-query-executor/internal/store"
	slacksdk "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

type approvedFakeSlackAPI struct {
	mu              sync.Mutex
	groupMembers    []string
	groupMembersErr error
}

type approvedFakeExecutor struct {
	mu        sync.Mutex
	callCount int
	returnErr error
}

func (f *approvedFakeSlackAPI) PostMessage(channelID string, options ...slacksdk.MsgOption) (string, string, error) {
	return "", "", nil
}

func (f *approvedFakeSlackAPI) PostMessageContext(ctx context.Context, channelID string, options ...slacksdk.MsgOption) (string, string, error) {
	return "", "", nil
}

func (f *approvedFakeSlackAPI) GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slacksdk.GetUserGroupMembersOption) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.groupMembers, f.groupMembersErr
}

func (f *approvedFakeSlackAPI) DeleteFileContext(ctx context.Context, fileID string) error {
	return nil
}

func (f *approvedFakeSlackAPI) DeleteMessage(channelID, timestamp string) (string, string, error) {
	return "", "", nil
}

func (f *approvedFakeSlackAPI) UploadFileContext(ctx context.Context, params slacksdk.UploadFileParameters) (*slacksdk.FileSummary, error) {
	return &slacksdk.FileSummary{}, nil
}

func (f *approvedFakeExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) (*executor.ResultFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return &executor.ResultFile{}, nil
}

func newApprovedEventForTest(api *approvedFakeSlackAPI, pending *store.PendingStore) *events.ApprovedEvent {
	cfg := &config.Config{
		ChannelID:       "C_CHANNEL",
		ApproverGroupID: "S_GROUP",
		ApproveEmoji:    "white_check_mark",
	}

	deps := depspkg.NewDeps(api, pending, nil, cfg, &approvedFakeExecutor{})
	return events.NewApprovedEvent(deps)
}

func buildApprovedReactionEvent(reaction, userID, itemChannel, itemTS, itemType string) *socketmode.Event {
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
	return &socketmode.Event{
		Data: slackevents.EventsAPIEvent{
			InnerEvent: slackevents.EventsAPIInnerEvent{Data: inner},
		},
	}
}

func approvedPendingEntry() store.PendingEntry {
	return store.PendingEntry{
		Channel:     "C_CHANNEL",
		MessageTS:   "1234567890.000001",
		SubmittedBy: "U_SUBMITTER",
		RawQuery:    `{"find": "users"}`,
		SubmittedAt: time.Now(),
	}
}

func TestHandleReaction_WrongEmoji(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_APPROVER"}}, pending)

	h.Process(buildApprovedReactionEvent("thumbsdown", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "message"))

	if _, ok := pending.Get("1234567890.000001"); !ok {
		t.Fatal("expected pending entry to remain for wrong emoji")
	}
}

func TestHandleReaction_WrongItemType(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_APPROVER"}}, pending)

	h.Process(buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "file"))

	if _, ok := pending.Get("1234567890.000001"); !ok {
		t.Fatal("expected pending entry to remain for wrong item type")
	}
}

func TestHandleReaction_WrongChannel(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_APPROVER"}}, pending)

	h.Process(buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_OTHER", "1234567890.000001", "message"))

	if _, ok := pending.Get("1234567890.000001"); !ok {
		t.Fatal("expected pending entry to remain for wrong channel")
	}
}

func TestHandleReaction_GroupAPIError(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembersErr: errors.New("slack API down")}, pending)

	h.Process(buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "message"))

	if _, ok := pending.Get("1234567890.000001"); !ok {
		t.Fatal("expected pending entry to remain when approver lookup fails")
	}
}

func TestHandleReaction_UserNotInGroup(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_OTHER_APPROVER"}}, pending)

	h.Process(buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "message"))

	if _, ok := pending.Get("1234567890.000001"); !ok {
		t.Fatal("expected pending entry to remain for non-approver")
	}
}

func TestHandleReaction_TSNotInStore(t *testing.T) {
	pending := store.New()
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_APPROVER"}}, pending)

	h.Process(buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "message"))

	if _, ok := pending.Get("1234567890.000001"); ok {
		t.Fatal("did not expect pending entry for missing timestamp")
	}
}

func TestHandleReaction_HappyPath(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_APPROVER"}}, pending)

	h.Process(buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "message"))

	if _, ok := pending.Get("1234567890.000001"); ok {
		t.Fatal("expected pending entry to be removed on approval")
	}
}

func TestHandleReaction_ConcurrentSameEvent(t *testing.T) {
	pending := store.New()
	pending.Set("1234567890.000001", approvedPendingEntry())
	h := newApprovedEventForTest(&approvedFakeSlackAPI{groupMembers: []string{"U_APPROVER"}}, pending)

	evt := buildApprovedReactionEvent("white_check_mark", "U_APPROVER", "C_CHANNEL", "1234567890.000001", "message")

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
			h.Process(evt)
		}()
	}
	wg.Wait()

	if _, ok := pending.Get("1234567890.000001"); ok {
		t.Fatal("expected pending entry to be removed after concurrent approvals")
	}
}
