package config

import (
	"log"
	"os"
	"time"
)

type Config struct {
	BotToken        string
	AppToken        string
	ChannelID       string
	ApproverGroupID string
	ApprovedUserIds string
	ApprovalMode    string
	ApproveEmoji    string
	MongoURI        string
	DBName          string
	MessageTTL      time.Duration
}

func Load() *Config {
	required := []struct {
		key   string
		field *string
	}{
		{"SLACK_BOT_TOKEN", nil},
		{"SLACK_APP_TOKEN", nil},
		{"SLACK_CHANNEL_ID", nil},
		{"SLACK_APPROVER_GROUP_ID", nil},
		{"SLACK_APPROVED_USER_IDS", nil},
		{"SLACK_APPROVAL_MODE", nil},
		{"MONGO_URI", nil},
		{"MONGO_DB_NAME", nil},
	}

	cfg := &Config{}
	required[0].field = &cfg.BotToken
	required[1].field = &cfg.AppToken
	required[2].field = &cfg.ChannelID
	required[3].field = &cfg.ApproverGroupID
	required[4].field = &cfg.ApprovedUserIds
	required[5].field = &cfg.ApprovalMode
	required[6].field = &cfg.MongoURI
	required[7].field = &cfg.DBName

	for _, r := range required {
		val := os.Getenv(r.key)
		if val == "" {
			log.Fatalf("required env var %s is not set", r.key)
		}
		*r.field = val
	}

	cfg.ApproveEmoji = os.Getenv("APPROVE_EMOJI")
	if cfg.ApproveEmoji == "" {
		cfg.ApproveEmoji = "white_check_mark"
	}

	if ttlStr := os.Getenv("MESSAGE_TTL"); ttlStr != "" {
		d, err := time.ParseDuration(ttlStr)
		if err != nil {
			log.Fatalf("invalid MESSAGE_TTL %q: %v", ttlStr, err)
		}
		cfg.MessageTTL = d
	}

	return cfg
}
