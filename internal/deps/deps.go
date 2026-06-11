package deps

import (
	"context"

	"github.com/rawmind/slack-query-executor/internal/config"
	"github.com/rawmind/slack-query-executor/internal/executor"
	"github.com/rawmind/slack-query-executor/internal/store"
	"github.com/slack-go/slack"
)

type SlackAPIClient interface {
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
	GetUserGroupMembersContext(ctx context.Context, userGroup string, options ...slack.GetUserGroupMembersOption) ([]string, error)
	DeleteFileContext(ctx context.Context, fileID string) error
	DeleteMessage(channelID, timestamp string) (string, string, error)
	UploadFileContext(ctx context.Context, params slack.UploadFileParameters) (file *slack.FileSummary, err error)
}

type Deps struct {
	slackApi     SlackAPIClient
	store        *store.PendingStore
	fileMsgStore *store.FileMsgStore
	config       *config.Config
	exec         executor.Executor
}

func NewDeps(api SlackAPIClient, pendingStore *store.PendingStore, fileMsgStore *store.FileMsgStore, cfg *config.Config, exec executor.Executor) Deps {
	return Deps{
		slackApi:     api,
		store:        pendingStore,
		fileMsgStore: fileMsgStore,
		config:       cfg,
		exec:         exec,
	}
}

func (d Deps) SlackAPI() SlackAPIClient {
	return d.slackApi
}

func (d Deps) Store() *store.PendingStore {
	return d.store
}

func (d Deps) FileMsgStore() *store.FileMsgStore {
	return d.fileMsgStore
}

func (d Deps) Config() *config.Config {
	return d.config
}

func (d Deps) Executor() executor.Executor {
	return d.exec
}
