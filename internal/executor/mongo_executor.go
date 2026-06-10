package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/rawmind/slack-query-executor/internal/parser"
	"github.com/rawmind/slack-query-executor/internal/store"
)

const defaultQueryTimeout = 10 * time.Second
const maxDocCap = 500

var reOID = regexp.MustCompile(`(?s)\{\s*"\$oid":\s*"([a-f0-9]{24})"\s*\}`)

type ResultMeta struct {
	ExecutedAt string `json:"executed_at"`
	RuntimeMS  int64  `json:"runtime_ms"`
	DocCount   int    `json:"doc_count"`
}

type ResultFile struct {
	Meta    ResultMeta        `json:"meta"`
	Results []json.RawMessage `json:"results"`
}

type MongoExecutor struct {
	api    *slack.Client
	client *mongo.Client
	dbName string
}

func NewMongoExecutor(api *slack.Client, client *mongo.Client, dbName string) *MongoExecutor {
	return &MongoExecutor{api: api, client: client, dbName: dbName}
}

func (e *MongoExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) error {
	q, err := parser.ParseShellQuery(entry.RawQuery)
	if err != nil {
		e.postError(ctx, entry, ":x: Query parse error: "+err.Error())
		return nil
	}

	slog.Info("Executing query", "ts", entry.MessageTS, "approver", approverID, "op", q.Op, "collection", q.Collection, "db", e.dbName)
	coll := e.client.Database(e.dbName).Collection(q.Collection)

	start := time.Now()
	queryCtx, cancel := context.WithTimeout(ctx, defaultQueryTimeout)
	defer cancel()

	var docs []bson.Raw
	switch q.Op {
	case "find":
		docs, err = runFind(queryCtx, coll, q)
	case "aggregate":
		docs, err = runAggregate(queryCtx, coll, q)
	default:

		e.postError(ctx, entry, ":x: Unsupported operation: "+q.Op)
		return nil
	}

	if err != nil {
		slog.Error("MongoDB query failed", "ts", entry.MessageTS, "err", err)
		e.postError(ctx, entry, ":x: Query execution failed: "+err.Error())
		return nil
	}

	elapsed := time.Since(start)
	truncated := false
	if len(docs) > maxDocCap {
		docs = docs[:maxDocCap]
		truncated = true
	}

	results, err := rawDocsToJSON(docs)
	if err != nil {
		slog.Error("BSON to JSON conversion failed", "ts", entry.MessageTS, "err", err)
		e.postError(ctx, entry, ":x: Failed to serialize query results: "+err.Error())
		return nil
	}

	rf := ResultFile{
		Meta: ResultMeta{
			ExecutedAt: time.Now().UTC().Format(time.RFC3339),
			RuntimeMS:  elapsed.Milliseconds(),
			DocCount:   len(results),
		},
		Results: results,
	}

	jsonBytes, _ := json.MarshalIndent(rf, "", "  ")
	jsonBytes = replaceOIDTokens(jsonBytes)
	ts := strings.ReplaceAll(entry.MessageTS, ".", "-")
	filename := fmt.Sprintf("query-results-%s.json", ts)
	initialComment := ""
	if truncated {
		initialComment = fmt.Sprintf("Results truncated to %d documents (soft cap).", maxDocCap)
	}

	_, err = e.api.UploadFileContext(ctx, slack.UploadFileParameters{
		Channel:         entry.Channel,
		ThreadTimestamp: entry.MessageTS,
		Filename:        filename,
		Title:           filename,
		Content:         string(jsonBytes),
		FileSize:        len(jsonBytes),
		InitialComment:  initialComment,
	})
	if err != nil {
		slog.Error("file upload failed", "ts", entry.MessageTS, "err", err)
		if isMissingScopeErr(err) {
			e.postError(ctx, entry,
				":x: Cannot upload result file because this Slack app is missing required scope `files:write` (Upload URL API returned `missing_scope`). "+
					"Ask an admin to add `files:write` in OAuth scopes and reinstall the app, then retry.")
			return nil
		}
		e.postError(ctx, entry, ":x: Failed to upload results: "+err.Error())
		return nil
	}

	slog.Info("query executed",
		"ts", entry.MessageTS,
		"approver", approverID,
		"doc_count", len(results),
		"runtime_ms", elapsed.Milliseconds(),
	)
	return nil
}

func (e *MongoExecutor) postError(ctx context.Context, entry store.PendingEntry, text string) {
	_, _, err := e.api.PostMessageContext(ctx, entry.Channel,
		slack.MsgOptionTS(entry.MessageTS),
		slack.MsgOptionText(text, false),
	)
	if err != nil {
		slog.Error("PostMessageContext failed", "err", err)
	}
}

func runFind(ctx context.Context, coll *mongo.Collection, q parser.ParsedQuery) ([]bson.Raw, error) {
	opts := options.Find()
	if q.FindOpts.Limit != nil {
		opts.SetLimit(*q.FindOpts.Limit)
	}
	if q.FindOpts.Sort != nil {
		opts.SetSort(q.FindOpts.Sort)
	}
	if q.FindOpts.Projection != nil {
		opts.SetProjection(q.FindOpts.Projection)
	}

	cur, err := coll.Find(ctx, q.Filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []bson.Raw
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func runAggregate(ctx context.Context, coll *mongo.Collection, q parser.ParsedQuery) ([]bson.Raw, error) {
	cur, err := coll.Aggregate(ctx, q.Pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []bson.Raw
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func rawDocsToJSON(docs []bson.Raw) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0, len(docs))
	for _, doc := range docs {
		b, err := bson.MarshalExtJSON(doc, false, false)
		if err != nil {
			return nil, fmt.Errorf("BSON marshal failed: %w", err)
		}
		out = append(out, json.RawMessage(b))
	}
	return out, nil
}

func isMissingScopeErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "missing_scope")
}

func replaceOIDTokens(b []byte) []byte {
	return reOID.ReplaceAll(b, []byte(`ObjectId("$1")`))
}
