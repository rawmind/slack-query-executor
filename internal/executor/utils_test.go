package executor

import "testing"

func TestReplaceOIDTokens(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single oid object",
			input: `{"_id":{"$oid":"507f1f77bcf86cd799439011"}}`,
			want:  `{"_id":ObjectId("507f1f77bcf86cd799439011")}`,
		},
		{
			name:  "multiple oid objects",
			input: `[{"$oid":"507f1f77bcf86cd799439011"},{"$oid":"507f1f77bcf86cd799439012"}]`,
			want:  `[ObjectId("507f1f77bcf86cd799439011"),ObjectId("507f1f77bcf86cd799439012")]`,
		},
		{
			name:  "uppercase oid stays unchanged",
			input: `{"$oid":"507F1F77BCF86CD799439011"}`,
			want:  `{"$oid":"507F1F77BCF86CD799439011"}`,
		},
		{
			name:  "non oid json unchanged",
			input: `{"name":"alice","age":30}`,
			want:  `{"name":"alice","age":30}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(ReplaceOIDTokens([]byte(tc.input)))
			if got != tc.want {
				t.Errorf("ReplaceOIDTokens(%q)\ngot:  %q\nwant: %q", tc.input, got, tc.want)
			}
		})
	}
}
