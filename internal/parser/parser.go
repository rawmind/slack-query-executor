package parser

import (
	"regexp"
	"strings"
)

var codeBlockRe = regexp.MustCompile("(?s)" + "```" + "(.*?)" + "```")

func ExtractCodeBlock(text string) (string, bool) {
	matches := codeBlockRe.FindStringSubmatch(text)
	if len(matches) < 2 {
		return "", false
	}

	content := strings.TrimSpace(matches[1])
	if content == "" {
		return "", false
	}

	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		firstLine := strings.TrimSpace(content[:idx])
		if firstLine != "" && !strings.ContainsAny(firstLine, " {[\"(") {
			content = strings.TrimSpace(content[idx+1:])
		}
	}

	if content == "" {
		return "", false
	}

	content = normalizeSlackQueryInput(content)
	content = strings.TrimSpace(content)

	return content, true
}
