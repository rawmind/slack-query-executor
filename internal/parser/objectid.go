package parser

import (
	"fmt"
	"regexp"
)

var objectIdRe = regexp.MustCompile(`ObjectId\("([^"]*)"\)`)

func convertObjectIdNotation(s string) (string, error) {
	if s == "" {
		return "", nil
	}

	var convErr error
	result := objectIdRe.ReplaceAllStringFunc(s, func(match string) string {
		if convErr != nil {

			return match
		}

		sub := objectIdRe.FindStringSubmatch(match)
		if len(sub) < 2 {

			convErr = fmt.Errorf("invalid ObjectId: failed to extract argument from %q", match)
			return match
		}
		val := sub[1]
		if len(val) != 24 {
			convErr = fmt.Errorf("invalid ObjectId: %q must be a 24-character hex string", val)
			return match
		}
		for _, c := range val {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				convErr = fmt.Errorf("invalid ObjectId: %q must be a 24-character hex string", val)
				return match
			}
		}
		return `{"$oid":"` + val + `"}`
	})

	if convErr != nil {
		return "", convErr
	}
	return result, nil
}
