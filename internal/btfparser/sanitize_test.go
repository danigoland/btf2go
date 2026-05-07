package btfparser

import "testing"

func TestSanitizeName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"MyEvent", "MyEvent"},
		{"my_module::MyEvent", "MyModuleMyEvent"},
		{"a::b::c::Foo", "ABCFoo"},
		{"events.Inner", "EventsInner"},
		{"foo-bar", "FooBar"},
		{"$weird name", "WeirdName"},
		{"!!!", "_anon"},
		{"123Bad", "_123Bad"},
		{"", "_anon"},
	}
	for _, c := range cases {
		if got := SanitizeName(c.in); got != c.want {
			t.Errorf("SanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAnonName(t *testing.T) {
	if got := AnonName("Outer", "data", 0); got != "OuterDataAnon0" {
		t.Fatalf("got %q", got)
	}
	if got := AnonName("", "", 3); got != "Anon3" {
		t.Fatalf("got %q", got)
	}
}
