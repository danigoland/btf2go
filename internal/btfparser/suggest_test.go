package btfparser

import "testing"

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
	// No suggestions: bare cause.
	bare := &TypeNotFoundError{
		Name:        "Foo",
		Suggestions: nil,
		Cause:       errString("type not found"),
	}
	if got := bare.Error(); got != `--type "Foo": type not found` {
		t.Fatalf("bare: got %q", got)
	}

	// With suggestions: includes "Did you mean".
	with := &TypeNotFoundError{
		Name:        "events_T",
		Suggestions: []string{"events_t", "EventsT"},
		Cause:       errString("type not found"),
	}
	got := with.Error()
	want := `--type "events_T" not found. Did you mean: [events_t EventsT]? (run ` + "`btf2go inspect`" + ` to list all named types)`
	if got != want {
		t.Fatalf("with: got %q, want %q", got, want)
	}

	// Cause is unwrapped.
	if with.Unwrap() == nil {
		t.Fatal("expected Unwrap to return cause")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
