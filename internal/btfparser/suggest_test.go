package btfparser

import (
	"errors"
	"testing"
)

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},   // substitution
		{"abc", "abcd", 1},  // insertion
		{"abcd", "abc", 1},  // deletion
		{"events_t", "events_T", 1},
		{"kitten", "sitting", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestTypeNotFoundErrorMessage(t *testing.T) {
	cause := errors.New("type not found")

	// No suggestions: bare cause.
	bare := &TypeNotFoundError{
		Name:        "Foo",
		Suggestions: nil,
		Cause:       cause,
	}
	if got := bare.Error(); got != `--type "Foo": type not found` {
		t.Fatalf("bare: got %q", got)
	}

	// With suggestions: includes "Did you mean".
	with := &TypeNotFoundError{
		Name:        "events_T",
		Suggestions: []string{"events_t", "EventsT"},
		Cause:       cause,
	}
	got := with.Error()
	want := `--type "events_T" not found. Did you mean: [events_t EventsT]? (run ` + "`btf2go inspect`" + ` to list all named types)`
	if got != want {
		t.Fatalf("with: got %q, want %q", got, want)
	}

	// errors.Is must reach the underlying cause through the wrapper.
	if !errors.Is(with, cause) {
		t.Fatal("errors.Is(with, cause) returned false; Unwrap broken")
	}
}
