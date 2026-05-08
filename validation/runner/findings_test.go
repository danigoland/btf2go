package main

import "testing"

func TestStatusString(t *testing.T) {
	cases := []struct {
		s    Status
		want string
	}{
		{StatusPass, "PASS"},
		{StatusFail, "FAIL"},
		{StatusSkip, "SKIP"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("Status(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestTierResultPassRate(t *testing.T) {
	t.Run("two-pass one-fail", func(t *testing.T) {
		r := TierResult{
			Tier: "T1",
			Findings: []Finding{
				{Status: StatusPass}, {Status: StatusPass}, {Status: StatusFail},
			},
		}
		got := r.PassRate()
		if got < 0.66 || got > 0.67 {
			t.Fatalf("pass rate = %f, want ~0.667", got)
		}
	})

	t.Run("all-skip", func(t *testing.T) {
		r := TierResult{
			Tier: "T1",
			Findings: []Finding{
				{Status: StatusSkip}, {Status: StatusSkip},
			},
		}
		got := r.PassRate()
		if got != 0 {
			t.Fatalf("pass rate = %f, want 0 for all-skip", got)
		}
	})
}
