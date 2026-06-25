package config

import (
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	BotToken        string
	AppToken        string
	ChannelID       string
	ApproverGroupID string
	ApprovedUserIDs []string
	ApproveEmoji    string
	MongoURI        string
	DBName          string
	MessageTTL      time.Duration
}

func Load() *Config {
	cfg := &Config{}
	cfg.BotToken = requiredEnv("SLACK_BOT_TOKEN")
	cfg.AppToken = requiredEnv("SLACK_APP_TOKEN")
	cfg.ChannelID = requiredEnv("SLACK_CHANNEL_ID")
	approvedUserDs := parseCommaSeparated(requiredEnvOrDefault("SLACK_APPROVED_USER_IDS", ""))
	approverGroupID := requiredEnvOrDefault("SLACK_APPROVER_GROUP_ID", "")
	if len(approvedUserDs) == 0 && approverGroupID == "" {
		log.Fatalf("either SLACK_APPROVED_USER_IDS or SLACK_APPROVER_GROUP_ID must be set")
	}
	cfg.ApproverGroupID = approverGroupID
	cfg.ApprovedUserIDs = approvedUserDs
	cfg.MongoURI = requiredEnv("MONGO_URI")
	cfg.DBName = requiredEnv("MONGO_DB_NAME")
	cfg.ApproveEmoji = requiredEnvOrDefault("SLACK_APPROVE_EMOJI", "white_check_mark")
	if ttlStr := os.Getenv("MESSAGE_TTL"); ttlStr != "" {
		d, err := time.ParseDuration(ttlStr)
		if err != nil {
			log.Fatalf("invalid MESSAGE_TTL %q: %v", ttlStr, err)
		}
		cfg.MessageTTL = d
	}
	return cfg
}

func parseCommaSeparated(s string) []string {
	var ids []string
	for id := range strings.SplitSeq(s, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func requiredEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return val
}

func requiredEnvOrDefault(key, defaultValue string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultValue
	}
	return val
}
