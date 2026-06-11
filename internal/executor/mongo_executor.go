package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/rawmind/slack-query-executor/internal/parser"
	"github.com/rawmind/slack-query-executor/internal/store"
)

const defaultQueryTimeout = 10 * time.Second
const maxDocCap = 500

type ResultMeta struct {
	ExecutedAt string `json:"executed_at"`
	RuntimeMS  int64  `json:"runtime_ms"`
	DocCount   int    `json:"doc_count"`
}

type ResultFile struct {
	Meta    ResultMeta        `json:"meta"`
	Results []json.RawMessage `json:"results"`
	Truncated bool              `json:"truncated,omitempty"`
	MaxDocCap int               `json:"-"`
}

type MongoExecutor struct {
	client       *mongo.Client
	dbName       string
}

func NewMongoExecutor(client *mongo.Client, dbName string) *MongoExecutor {
	return &MongoExecutor{client: client, dbName: dbName}
}

func (e *MongoExecutor) Execute(ctx context.Context, entry store.PendingEntry, approverID string) (*ResultFile, error) {
	q, err := parser.ParseShellQuery(entry.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("query parse error: %w", err)
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
		return nil, fmt.Errorf("unsupported operation: %s", q.Op)
	}

	if err != nil {
		slog.Error("MongoDB query failed", "ts", entry.MessageTS, "err", err)
		return nil, err
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
		return nil, fmt.Errorf(":x: Failed to serialize query results: %w", err)
	}

	rf := ResultFile{
		Meta: ResultMeta{
			ExecutedAt: time.Now().UTC().Format(time.RFC3339),
			RuntimeMS:  elapsed.Milliseconds(),
			DocCount:   len(results),
		},
		Results: results,
		Truncated: truncated,
		MaxDocCap: maxDocCap,
	}

	slog.Info("query executed",
		"ts", entry.MessageTS,
		"approver", approverID,
		"doc_count", len(results),
		"runtime_ms", elapsed.Milliseconds(),
	)
	return &rf, nil
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
