package main

import (
	"context"
	"log"
	"log/slog"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/rawmind/slack-query-executor/internal/config"
	"github.com/rawmind/slack-query-executor/internal/executor"
	"github.com/rawmind/slack-query-executor/internal/handler"
)

func main() {
	cfg := config.Load()
	api := slack.New(
		cfg.BotToken,
		slack.OptionAppLevelToken(cfg.AppToken),
	)
	authResp, err := api.AuthTest()
	if err != nil {
		log.Fatalf("Slack AuthTest failed: %v", err)
	}
	botUserID := authResp.UserID

	slog.Info("connected to Slack", "bot_user_id", botUserID)
	slog.Info("runtime config",
		"channel_id", cfg.ChannelID,
		"approver_group_id", cfg.ApproverGroupID,
		"approve_emoji", cfg.ApproveEmoji,
	)
	mongoClient, err := mongo.Connect(options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatalf("MongoDB connect failed: %v", err)
	}
	defer func() {
		if err := mongoClient.Disconnect(context.Background()); err != nil {
			slog.Error("MongoDB disconnect error", "err", err)
		}
	}()

	slog.Info("MongoDB client created", "db", cfg.DBName)
	client := socketmode.New(api)
	exec := executor.NewMongoExecutor(api, mongoClient, cfg.DBName)
	h := handler.New(api, client, cfg.ChannelID, botUserID, cfg.ApproverGroupID, cfg.ApproveEmoji, exec)
	h.Register()

	if err := h.Run(); err != nil {
		log.Fatalf("event loop error: %v", err)
	}
}
