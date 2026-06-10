package parser

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var slackQueryNormalizer = strings.NewReplacer(
	"\u201c", `"`,
	"\u201d", `"`,
	"\u2018", `'`,
	"\u2019", `'`,
	"\u00a0", " ",
)

var slackMailtoRe = regexp.MustCompile(`<mailto:([^>|]+)(?:\|[^>]+)?>`)

var shellQueryRe = regexp.MustCompile(`^db\.([A-Za-z_][A-Za-z0-9_\-]*)\.([A-Za-z]+)\(([\s\S]*)\)$`)

type ParsedQuery struct {
	Collection string
	Op         string
	Filter     interface{}
	FindOpts   FindOptions
	Pipeline   interface{}
}

type FindOptions struct {
	Limit      *int64
	Sort       interface{}
	Projection interface{}
}

type findOptionsRaw struct {
	Limit      *float64    `json:"limit"`
	Sort       interface{} `json:"sort"`
	Projection interface{} `json:"projection"`
}

func ParseShellQuery(raw string) (ParsedQuery, error) {
	raw = normalizeSlackQueryInput(raw)
	var err error
	raw, err = convertObjectIdNotation(raw)
	if err != nil {
		return ParsedQuery{}, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedQuery{}, fmt.Errorf("query is empty")
	}

	m := shellQueryRe.FindStringSubmatch(raw)
	if m == nil {
		return ParsedQuery{}, fmt.Errorf("invalid query format: expected db.<collection>.<operation>(<args>) with strict JSON arguments")
	}

	collection := m[1]
	op := strings.ToLower(m[2])
	argsStr := strings.TrimSpace(m[3])

	if containsChaining(argsStr) {
		return ParsedQuery{}, fmt.Errorf("chained method calls (.limit(), .sort(), etc.) are not supported — use find options object instead")
	}

	slog.Debug("Parsed query", "collection", collection, "operation", op, "args", argsStr, "original", raw)

	switch op {
	case "find":
		return parseFindQuery(collection, argsStr)
	case "aggregate":
		return parseAggregateQuery(collection, argsStr)
	default:
		return ParsedQuery{}, fmt.Errorf("unsupported operation %q: only find and aggregate are allowed", op)
	}
}

func normalizeSlackQueryInput(raw string) string {
	raw = slackQueryNormalizer.Replace(raw)
	raw = slackMailtoRe.ReplaceAllString(raw, "$1")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u200b', '\u200c', '\u200d', '\ufeff', '\u2060':
			return -1
		default:
			return r
		}
	}, raw)
}

func parseFindQuery(collection, argsStr string) (ParsedQuery, error) {
	first, second, err := splitTopLevelArgs(argsStr)
	if err != nil {
		return ParsedQuery{}, fmt.Errorf("failed to split find arguments: %w", err)
	}

	var filter interface{}
	if err := json.Unmarshal([]byte(first), &filter); err != nil {
		return ParsedQuery{}, fmt.Errorf("invalid JSON in find filter: %v — strict JSON required (double-quoted keys, no comments)", err)
	}
	if err := checkFilterSafety(filter); err != nil {
		return ParsedQuery{}, err
	}

	var bsonFilter bson.M
	if err := bson.UnmarshalExtJSON([]byte(first), false, &bsonFilter); err != nil {
		return ParsedQuery{}, fmt.Errorf("invalid extended JSON in find filter: %v", err)
	}

	var opts FindOptions
	if second != "" {
		var raw findOptionsRaw
		if err := json.Unmarshal([]byte(second), &raw); err != nil {
			return ParsedQuery{}, fmt.Errorf("invalid JSON in find options: %v", err)
		}
		if raw.Limit != nil {
			f := *raw.Limit
			if f < 0 {
				return ParsedQuery{}, fmt.Errorf("limit must be non-negative, got %g", f)
			}
			if f != math.Trunc(f) {
				return ParsedQuery{}, fmt.Errorf("limit must be an integer, got %g", f)
			}
			v := int64(f)
			opts.Limit = &v
		}
		opts.Sort = raw.Sort
		opts.Projection = raw.Projection
	}

	return ParsedQuery{
		Collection: collection,
		Op:         "find",
		Filter:     bsonFilter,
		FindOpts:   opts,
	}, nil
}

func parseAggregateQuery(collection, argsStr string) (ParsedQuery, error) {
	var pipeline []interface{}
	if err := json.Unmarshal([]byte(argsStr), &pipeline); err != nil {
		return ParsedQuery{}, fmt.Errorf("invalid JSON in aggregate pipeline: %v — pipeline must be a JSON array", err)
	}
	if err := checkPipelineSafety(pipeline); err != nil {
		return ParsedQuery{}, err
	}

	var bsonPipeline bson.A
	if err := bson.UnmarshalExtJSON([]byte(argsStr), false, &bsonPipeline); err != nil {
		return ParsedQuery{}, fmt.Errorf("invalid extended JSON in aggregate pipeline: %v", err)
	}

	return ParsedQuery{
		Collection: collection,
		Op:         "aggregate",
		Pipeline:   bsonPipeline,
	}, nil
}

func containsChaining(s string) bool {
	depth := 0
	started := false
	for i, ch := range s {
		switch ch {
		case '{', '[', '(':
			depth++
			started = true
		case '}', ']', ')':
			if depth > 0 {
				depth--
			}
		}
		if started && depth == 0 {
			rest := s[i+1:]

			rest = strings.TrimLeft(rest, ")")
			rest = strings.TrimSpace(rest)
			if strings.HasPrefix(rest, ".") {
				return true
			}
		}
	}
	return false
}

func splitTopLevelArgs(s string) (first, second string, err error) {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			}

		case ',':
			if depth == 0 {
				return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), nil
			}
		}
	}
	return strings.TrimSpace(s), "", nil
}
