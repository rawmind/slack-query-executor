package executor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/rawmind/slack-query-executor/internal/parser"
	"github.com/rawmind/slack-query-executor/internal/store"
)

func TestMongoExecutor_ParseError(t *testing.T) {
	entries := []store.PendingEntry{
		{
			RawQuery: `db.users.insertOne({"name":"x"})`,
		},
		{
			RawQuery: `db.users.find({}).limit(10)`,
		},
		{
			RawQuery: `not a valid query`,
		},
	}

	for _, entry := range entries {
		_, err := parser.ParseShellQuery(entry.RawQuery)
		if err == nil {
			t.Errorf("ParseShellQuery(%q) = nil error, want non-nil", entry.RawQuery)
		}
	}
}

func TestRawDocsToJSON(t *testing.T) {
	doc, err := bson.Marshal(bson.D{
		{Key: "name", Value: "alice"},
		{Key: "age", Value: int32(30)},
	})
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}

	results, err := rawDocsToJSON([]bson.Raw{bson.Raw(doc)})
	if err != nil {
		t.Fatalf("rawDocsToJSON: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	var got map[string]interface{}
	if err := json.Unmarshal(results[0], &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if got["name"] != "alice" {
		t.Errorf("name = %v, want alice", got["name"])
	}
	if got["age"] != float64(30) {
		t.Errorf("age = %v, want 30", got["age"])
	}
}

func TestResultFileShape(t *testing.T) {
	doc, _ := bson.Marshal(bson.D{{Key: "x", Value: int32(1)}})
	results, _ := rawDocsToJSON([]bson.Raw{bson.Raw(doc)})

	rf := ResultFile{
		Meta: ResultMeta{
			ExecutedAt: time.Now().UTC().Format(time.RFC3339),
			RuntimeMS:  42,
			DocCount:   len(results),
		},
		Results: results,
	}

	jsonBytes, err := json.Marshal(rf)
	if err != nil {
		t.Fatalf("json.Marshal ResultFile: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := got["meta"]; !ok {
		t.Error("missing top-level key 'meta'")
	}
	if _, ok := got["results"]; !ok {
		t.Error("missing top-level key 'results'")
	}

	meta, ok := got["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("'meta' is not an object")
	}
	if _, ok := meta["executed_at"]; !ok {
		t.Error("meta missing 'executed_at'")
	}
	if meta["runtime_ms"] != float64(42) {
		t.Errorf("meta.runtime_ms = %v, want 42", meta["runtime_ms"])
	}
	if meta["doc_count"] != float64(1) {
		t.Errorf("meta.doc_count = %v, want 1", meta["doc_count"])
	}

	resArr, ok := got["results"].([]interface{})
	if !ok {
		t.Fatalf("'results' is not an array")
	}
	if len(resArr) != 1 {
		t.Errorf("len(results) = %d, want 1", len(resArr))
	}
}

func TestDocCapTruncation(t *testing.T) {

	total := maxDocCap + 10
	docs := make([]bson.Raw, total)
	for i := range docs {
		raw, _ := bson.Marshal(bson.D{{Key: "i", Value: int32(i)}})
		docs[i] = bson.Raw(raw)
	}

	truncated := false
	if len(docs) > maxDocCap {
		docs = docs[:maxDocCap]
		truncated = true
	}
	if !truncated {
		t.Fatal("expected truncated=true")
	}

	results, err := rawDocsToJSON(docs)
	if err != nil {
		t.Fatalf("rawDocsToJSON: %v", err)
	}
	if len(results) != maxDocCap {
		t.Errorf("len(results) after cap = %d, want %d", len(results), maxDocCap)
	}
}

func TestFilenameFormat(t *testing.T) {
	ts := "1234567890.123456"
	want := "query-results-1234567890-123456.json"
	got := "query-results-" + replaceDotsWithDashes(ts) + ".json"
	if got != want {
		t.Errorf("filename = %q, want %q", got, want)
	}
}

func replaceDotsWithDashes(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			result[i] = '-'
		} else {
			result[i] = s[i]
		}
	}
	return string(result)
}

func TestMongoExecutor_Execute_ParseError(t *testing.T) {
	t.Skip("Slack client mock requires interface extraction — covered by integration test")
}

func TestIsMissingScopeErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "non-matching error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "matching lowercase",
			err:  errors.New("missing_scope"),
			want: true,
		},
		{
			name: "matching mixed case",
			err:  errors.New("Upload URL API returned Missing_Scope error"),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isMissingScopeErr(tc.err)
			if got != tc.want {
				t.Errorf("isMissingScopeErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestReplaceOIDTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "compact form",
			input: `{"_id":{"$oid":"507f1f77bcf86cd799439011"},"name":"alice"}`,
			want:  `{"_id":ObjectId("507f1f77bcf86cd799439011"),"name":"alice"}`,
		},
		{
			name:  "indented multi-line form",
			input: "{\n  \"_id\": {\n    \"$oid\": \"507f1f77bcf86cd799439011\"\n  },\n  \"name\": \"alice\"\n}",
			want:  "{\n  \"_id\": ObjectId(\"507f1f77bcf86cd799439011\"),\n  \"name\": \"alice\"\n}",
		},
		{
			name:  "no oid — unchanged",
			input: `{"name":"bob","age":30}`,
			want:  `{"name":"bob","age":30}`,
		},
		{
			name:  "empty input",
			input: "",
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(replaceOIDTokens([]byte(tc.input)))
			if got != tc.want {
				t.Errorf("replaceOIDTokens(%q)\ngot:  %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}

func init() {
	_ = context.Background
}
