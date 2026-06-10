package parser

import "testing"

func TestExtractCodeBlock(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOk bool
	}{
		{
			name:   "valid single-line",
			input:  "```{\"find\":\"users\"}```",
			want:   `{"find":"users"}`,
			wantOk: true,
		},
		{
			name:   "valid multi-line",
			input:  "```\n{\"aggregate\":\"orders\",\"pipeline\":[]}\n```",
			want:   `{"aggregate":"orders","pipeline":[]}`,
			wantOk: true,
		},
		{
			name:   "language hint stripped",
			input:  "```json\n{\"find\":\"users\"}\n```",
			want:   `{"find":"users"}`,
			wantOk: true,
		},
		{
			name:   "no code block",
			input:  "no code block here",
			want:   "",
			wantOk: false,
		},
		{
			name:   "empty block",
			input:  "```   ```",
			want:   "",
			wantOk: false,
		},
		{
			name:   "slack mailto wrapper normalized to plain email",
			input:  "```db.physicians.find({\"email\": \"<mailto:andrei@some.com|andrei@some.com>\"})```",
			want:   `db.physicians.find({"email": "andrei@some.com"})`,
			wantOk: true,
		},
		{
			name:   "zero width chars removed from extracted text",
			input:  "```db.physicians.find({\"email\": \"andrei\u200b@some.com\"})```",
			want:   `db.physicians.find({"email": "andrei@some.com"})`,
			wantOk: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ExtractCodeBlock(tc.input)
			if ok != tc.wantOk {
				t.Errorf("ExtractCodeBlock(%q) ok = %v, want %v", tc.input, ok, tc.wantOk)
			}
			if got != tc.want {
				t.Errorf("ExtractCodeBlock(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
