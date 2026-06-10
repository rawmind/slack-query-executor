package config

import (
	"log"
	"os"
)

type Config struct {
	BotToken        string
	AppToken        string
	ChannelID       string
	ApproverGroupID string
	ApproveEmoji    string
	MongoURI        string
	DBName          string
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
		{"MONGO_URI", nil},
		{"MONGO_DB_NAME", nil},
	}

	cfg := &Config{}
	required[0].field = &cfg.BotToken
	required[1].field = &cfg.AppToken
	required[2].field = &cfg.ChannelID
	required[3].field = &cfg.ApproverGroupID
	required[4].field = &cfg.MongoURI
	required[5].field = &cfg.DBName

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

	return cfg
}
