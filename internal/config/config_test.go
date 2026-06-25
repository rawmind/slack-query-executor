package config

import (
	"reflect"
	"testing"
)

func TestParseCommaSeparated_TrimAndSkipEmpty(t *testing.T) {
	input := " u1, ,u2 ,,  u3  ,"
	got := parseCommaSeparated(input)
	want := []string{"u1", "u2", "u3"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCommaSeparated(%q) = %#v, want %#v", input, got, want)
	}
}

func TestParseCommaSeparated_EmptyInput(t *testing.T) {
	got := parseCommaSeparated("")
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %#v", got)
	}
}

func TestRequiredEnvOrDefault_UsesDefaultWhenUnset(t *testing.T) {
	t.Setenv("CFG_TEST_KEY", "")

	got := requiredEnvOrDefault("CFG_TEST_KEY", "fallback")
	if got != "fallback" {
		t.Fatalf("requiredEnvOrDefault returned %q, want %q", got, "fallback")
	}
}

func TestRequiredEnvOrDefault_UsesEnvWhenSet(t *testing.T) {
	t.Setenv("CFG_TEST_KEY", "from-env")

	got := requiredEnvOrDefault("CFG_TEST_KEY", "fallback")
	if got != "from-env" {
		t.Fatalf("requiredEnvOrDefault returned %q, want %q", got, "from-env")
	}
}
