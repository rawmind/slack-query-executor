package events_test

import (
	"context"
	"testing"

	"github.com/rawmind/slack-query-executor/internal/config"
	depspkg "github.com/rawmind/slack-query-executor/internal/deps"
	"github.com/rawmind/slack-query-executor/internal/events"
	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
)

type requestFakeSlackAPI struct {
	postMessages []string
}

func (f *requestFakeSlackAPI) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	f.postMessages = append(f.postMessages, channelID)
	return "", "", nil
}

func (f *requestFakeSlackAPI) PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error) {
	return "", "", nil
}

func (f *requestFakeSlackAPI) GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slack.GetUserGroupMembersOption) ([]string, error) {
	return nil, nil
}

func (f *requestFakeSlackAPI) DeleteFileContext(ctx context.Context, fileID string) error {
	return nil
}

func (f *requestFakeSlackAPI) DeleteMessage(channelID, timestamp string) (string, string, error) {
	return "", "", nil
}

func (f *requestFakeSlackAPI) UploadFileContext(ctx context.Context, params slack.UploadFileParameters) (*slack.FileSummary, error) {
	return &slack.FileSummary{}, nil
}

func newRequestReceivedEventForTest(api depspkg.SlackAPIClient, pending *store.PendingStore) *events.RequestReceivedEvent {
	cfg := &config.Config{ApproveEmoji: "white_check_mark"}
	d := depspkg.NewDeps(api, pending, nil, cfg, nil)
	return events.NewRequestReceivedEvent(d)
}

func TestRequestReceived_StoresQueryAndAcksOnce(t *testing.T) {
	api := &requestFakeSlackAPI{}
	pending := store.New()
	h := newRequestReceivedEventForTest(api, pending)

	h.Process("```db.users.find({})```", "C_CHANNEL", "U_APPROVER", "1234567890.000001")

	if len(api.postMessages) != 1 {
		t.Fatalf("expected 1 acknowledgment post, got %d", len(api.postMessages))
	}
	if _, ok := pending.Delete("1234567890.000001"); !ok {
		t.Fatal("expected pending entry in store after request received")
	}
}

func TestRequestReceived_DuplicateDoesNotDoubleAck(t *testing.T) {
	api := &requestFakeSlackAPI{}
	pending := store.New()
	h := newRequestReceivedEventForTest(api, pending)

	text := "```db.users.find({})```"
	h.Process(text, "C_CHANNEL", "U_APPROVER", "1234567890.000001")
	h.Process(text, "C_CHANNEL", "U_APPROVER", "1234567890.000001")

	if len(api.postMessages) != 1 {
		t.Fatalf("expected exactly 1 acknowledgment post for duplicate ts, got %d", len(api.postMessages))
	}
}

func TestRequestReceived_NoCodeBlockRepliesError(t *testing.T) {
	api := &requestFakeSlackAPI{}
	pending := store.New()
	h := newRequestReceivedEventForTest(api, pending)

	h.Process("please run this query for me", "C_CHANNEL", "U_APPROVER", "1234567890.000001")

	if len(api.postMessages) != 1 {
		t.Fatalf("expected 1 error reply post for missing code block, got %d", len(api.postMessages))
	}
}

func TestRequestReceived_EmptyCodeBlockRepliesError(t *testing.T) {
	api := &requestFakeSlackAPI{}
	pending := store.New()
	h := newRequestReceivedEventForTest(api, pending)

	h.Process("```   ```", "C_CHANNEL", "U_APPROVER", "1234567890.000001")

	if len(api.postMessages) != 1 {
		t.Fatalf("expected 1 error reply post for empty code block, got %d", len(api.postMessages))
	}
}
