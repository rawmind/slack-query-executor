package executor

import "regexp"

var reOID = regexp.MustCompile(`(?s)\{\s*"\$oid":\s*"([a-f0-9]{24})"\s*\}`)

func ReplaceOIDTokens(b []byte) []byte {
	return reOID.ReplaceAll(b, []byte(`ObjectId("$1")`))
}

