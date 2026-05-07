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
	cases := []struct {
		parent, field string
		n             int
		want          string
	}{
		{"Outer", "data", 0, "OuterDataAnon0"},
		{"", "", 3, "Anon3"},
		// Special chars in parent/field must be sanitized before composing.
		{"my::mod", "field-1", 0, "MyModField1Anon0"},
		{"", "evt.kind", 7, "EvtKindAnon7"},
	}
	for _, c := range cases {
		if got := AnonName(c.parent, c.field, c.n); got != c.want {
			t.Errorf("AnonName(%q, %q, %d) = %q, want %q", c.parent, c.field, c.n, got, c.want)
		}
	}
}
