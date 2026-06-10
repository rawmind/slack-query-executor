package parser

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestParseShellQuery(t *testing.T) {

	i64 := func(v int64) *int64 { return &v }

	tests := []struct {
		name      string
		input     string
		wantOp    string
		wantColl  string
		wantErr   string
		wantLimit *int64
	}{
		{
			name:     "valid find simple",
			input:    `db.users.find({"active": true})`,
			wantOp:   "find",
			wantColl: "users",
		},
		{
			name:     "smart quotes normalized",
			input:    "db.users.find({\u201cactive\u201d: true})",
			wantOp:   "find",
			wantColl: "users",
		},
		{
			name:     "zero width chars removed",
			input:    "db.physicians.find({\"email\": \"andrei\u200b@rawmind.com\"})",
			wantOp:   "find",
			wantColl: "physicians",
		},
		{
			name:      "valid find with options limit",
			input:     `db.users.find({"active": true}, {"limit": 100})`,
			wantOp:    "find",
			wantColl:  "users",
			wantLimit: i64(100),
		},
		{
			name:     "valid aggregate",
			input:    `db.orders.aggregate([{"$match":{"status":"open"}}])`,
			wantOp:   "aggregate",
			wantColl: "orders",
		},
		{
			name:    "chaining rejected",
			input:   `db.users.find({}).limit(10)`,
			wantErr: "chained",
		},
		{
			name:    "unsupported operation insertOne",
			input:   `db.users.insertOne({"name":"x"})`,
			wantErr: "unsupported operation",
		},
		{
			name:    "relaxed JSON syntax rejected",
			input:   `db.users.find({name: 'alice'})`,
			wantErr: "JSON",
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: "empty",
		},
		{
			name:     "nested object does not break split",
			input:    `db.users.find({"nested": {"a": 1, "b": 2}})`,
			wantOp:   "find",
			wantColl: "users",
		},

		{
			name:    "SAFE-01 $out blocked",
			input:   `db.c.aggregate([{"$out":"target"}])`,
			wantErr: "$out",
		},
		{
			name:    "$merge blocked",
			input:   `db.c.aggregate([{"$merge":{"into":"t"}}])`,
			wantErr: "$merge",
		},

		{
			name:    "SAFE-02 $where in find top-level",
			input:   `db.c.find({"$where":"this.x>0"})`,
			wantErr: "$where",
		},
		{
			name:    "SAFE-02 $where nested in find",
			input:   `db.c.find({"$expr":{"$where":"1"}})`,
			wantErr: "$where",
		},
		{
			name:    "SAFE-02 $function in pipeline stage",
			input:   `db.c.aggregate([{"$project":{"x":{"$function":{"body":"","args":[],"lang":"js"}}}}])`,
			wantErr: "$function",
		},
		{
			name:    "SAFE-02 $accumulator in pipeline",
			input:   `db.c.aggregate([{"$group":{"x":{"$accumulator":{}}}}])`,
			wantErr: "$accumulator",
		},

		{
			name:    "WR-01 fractional limit rejected",
			input:   `db.c.find({}, {"limit": 1.5})`,
			wantErr: "integer",
		},

		{
			name:      "WR-02 string value with unmatched close brace",
			input:     `db.c.find({"key": "close}noopen"}, {"limit": 5})`,
			wantOp:    "find",
			wantColl:  "c",
			wantLimit: i64(5),
		},

		{
			name:    "CR-01 $out in $lookup sub-pipeline",
			input:   `db.c.aggregate([{"$lookup":{"from":"col","pipeline":[{"$out":"evil"}],"as":"x"}}])`,
			wantErr: "$out",
		},
		{
			name:    "CR-01 $merge in $unionWith sub-pipeline",
			input:   `db.c.aggregate([{"$unionWith":{"coll":"col","pipeline":[{"$merge":{"into":"evil"}}]}}])`,
			wantErr: "$merge",
		},
		{
			name:    "CR-01 $out in $facet sub-pipeline",
			input:   `db.c.aggregate([{"$facet":{"branch":[{"$out":"evil"}]}}])`,
			wantErr: "$out",
		},

		{
			name:    "SAFE-03 string stage",
			input:   `db.c.aggregate(["not-a-stage"])`,
			wantErr: "must be an object",
		},
		{
			name:    "SAFE-03 null stage",
			input:   `db.c.aggregate([null])`,
			wantErr: "must be an object",
		},

		{
			name:     "valid find unaffected by safety checks",
			input:    `db.c.find({"active":true})`,
			wantOp:   "find",
			wantColl: "c",
		},
		{
			name:     "valid aggregate unaffected by safety checks",
			input:    `db.c.aggregate([{"$match":{"status":"open"}}])`,
			wantOp:   "aggregate",
			wantColl: "c",
		},

		{
			name:     "ObjectId in find filter converted",
			input:    `db.users.find({"_id": ObjectId("507f1f77bcf86cd799439011")})`,
			wantOp:   "find",
			wantColl: "users",
		},
		{
			name:     "ObjectId in aggregate pipeline converted",
			input:    `db.orders.aggregate([{"$match":{"userId": ObjectId("507f1f77bcf86cd799439011")}}])`,
			wantOp:   "aggregate",
			wantColl: "orders",
		},
		{
			name:     "multiple ObjectIds in one query all converted",
			input:    `db.orders.aggregate([{"$match":{"userId": ObjectId("507f1f77bcf86cd799439011"), "orderId": ObjectId("507f1f77bcf86cd799439012")}}])`,
			wantOp:   "aggregate",
			wantColl: "orders",
		},
		{
			name:     "query with no ObjectId unaffected",
			input:    `db.users.find({"active": true})`,
			wantOp:   "find",
			wantColl: "users",
		},
		{
			name:    "ObjectId with non-hex argument rejected",
			input:   `db.users.find({"_id": ObjectId("notahex!!")})`,
			wantErr: "invalid ObjectId",
		},
		{
			name:    "ObjectId with 23-char hex rejected",
			input:   `db.users.find({"_id": ObjectId("507f1f77bcf86cd79943901")})`,
			wantErr: "24-character",
		},
		{
			name:    "ObjectId with 25-char hex rejected",
			input:   `db.users.find({"_id": ObjectId("507f1f77bcf86cd7994390111")})`,
			wantErr: "24-character",
		},
		{
			name:    "objectId wrong casing not converted passes through to JSON error",
			input:   `db.users.find({"_id": objectId("507f1f77bcf86cd799439011")})`,
			wantErr: "invalid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseShellQuery(tc.input)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseShellQuery(%q) expected error containing %q, got nil", tc.input, tc.wantErr)
				}
				if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
					t.Errorf("ParseShellQuery(%q) error = %q, want it to contain %q", tc.input, err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseShellQuery(%q) unexpected error: %v", tc.input, err)
			}

			if got.Op != tc.wantOp {
				t.Errorf("Op = %q, want %q", got.Op, tc.wantOp)
			}
			if got.Collection != tc.wantColl {
				t.Errorf("Collection = %q, want %q", got.Collection, tc.wantColl)
			}

			if tc.wantLimit != nil {
				if got.FindOpts.Limit == nil {
					t.Errorf("FindOpts.Limit = nil, want %d", *tc.wantLimit)
				} else if *got.FindOpts.Limit != *tc.wantLimit {
					t.Errorf("FindOpts.Limit = %d, want %d", *got.FindOpts.Limit, *tc.wantLimit)
				}
			}

			if got.Op == "find" && got.Filter == nil {
				t.Errorf("find query has nil Filter")
			}

			if got.Op == "aggregate" && got.Pipeline == nil {
				t.Errorf("aggregate query has nil Pipeline")
			}
		})
	}

	t.Run("depth cap exceeded", func(t *testing.T) {
		nested := `{"x":1}`
		for i := 0; i < 51; i++ {
			nested = `{"a":` + nested + `}`
		}
		input := `db.c.find(` + nested + `)`
		_, err := ParseShellQuery(input)
		if err == nil {
			t.Fatal("expected error for deeply nested input, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "nesting too deep") {
			t.Errorf("error = %q, want substring %q", err.Error(), "nesting too deep")
		}
	})

	t.Run("ObjectId in find filter decoded to bson.ObjectID", func(t *testing.T) {
		got, err := ParseShellQuery(`db.users.find({"_id": ObjectId("507f1f77bcf86cd799439011")})`)
		if err != nil {
			t.Fatalf("ParseShellQuery unexpected error: %v", err)
		}

		filter, ok := got.Filter.(bson.M)
		if !ok {
			t.Fatalf("Filter type = %T, want bson.M", got.Filter)
		}

		oid, ok := filter["_id"].(bson.ObjectID)
		if !ok {
			t.Fatalf("filter _id type = %T, want bson.ObjectID", filter["_id"])
		}
		if oid.Hex() != "507f1f77bcf86cd799439011" {
			t.Fatalf("_id hex = %q, want %q", oid.Hex(), "507f1f77bcf86cd799439011")
		}
	})

	t.Run("ObjectId in aggregate pipeline decoded to bson.ObjectID", func(t *testing.T) {
		got, err := ParseShellQuery(`db.orders.aggregate([{"$match":{"userId": ObjectId("507f1f77bcf86cd799439011")}}])`)
		if err != nil {
			t.Fatalf("ParseShellQuery unexpected error: %v", err)
		}

		pipeline, ok := got.Pipeline.(bson.A)
		if !ok {
			t.Fatalf("Pipeline type = %T, want bson.A", got.Pipeline)
		}
		if len(pipeline) != 1 {
			t.Fatalf("pipeline length = %d, want 1", len(pipeline))
		}

		stage, ok := pipeline[0].(bson.D)
		if !ok {
			t.Fatalf("stage type = %T, want bson.D", pipeline[0])
		}

		var match bson.D
		for _, elem := range stage {
			if elem.Key == "$match" {
				v, ok := elem.Value.(bson.D)
				if !ok {
					t.Fatalf("$match type = %T, want bson.D", elem.Value)
				}
				match = v
				break
			}
		}
		if match == nil {
			t.Fatal("$match stage not found")
		}

		var userIDValue interface{}
		for _, elem := range match {
			if elem.Key == "userId" {
				userIDValue = elem.Value
				break
			}
		}
		if userIDValue == nil {
			t.Fatal("userId key not found in $match")
		}
		oid, ok := userIDValue.(bson.ObjectID)
		if !ok {
			t.Fatalf("userId type = %T, want bson.ObjectID", userIDValue)
		}
		if oid.Hex() != "507f1f77bcf86cd799439011" {
			t.Fatalf("userId hex = %q, want %q", oid.Hex(), "507f1f77bcf86cd799439011")
		}
	})
}

func TestNormalizeSlackQueryInput(t *testing.T) {
	t.Run("mailto wrapper unwrapped", func(t *testing.T) {
		in := `db.physicians.find({"email": "<mailto:andrei@some.com|andrei@some.com>"})`
		got := normalizeSlackQueryInput(in)
		want := `db.physicians.find({"email": "andrei@some.com"})`
		if got != want {
			t.Fatalf("normalizeSlackQueryInput(%q) = %q, want %q", in, got, want)
		}
	})

	t.Run("zero width chars removed", func(t *testing.T) {
		in := "db.physicians.find({\"email\": \"andrei\u200b@some.com\"})"
		got := normalizeSlackQueryInput(in)
		want := `db.physicians.find({"email": "andrei@some.com"})`
		if got != want {
			t.Fatalf("normalizeSlackQueryInput(%q) = %q, want %q", in, got, want)
		}
	})
}
