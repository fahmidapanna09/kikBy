package auth

import "testing"

func TestIsValidE164(t *testing.T) {
	cases := []struct {
		phone string
		want  bool
	}{
		{"+8801712345678", true},
		{"+14155552671", true},
		{"01712345678", false}, // missing +
		{"+0123456789", false}, // leading zero after +
		{"+1", false},          // too short
		{"not-a-number", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isValidE164(c.phone); got != c.want {
			t.Errorf("isValidE164(%q) = %v, want %v", c.phone, got, c.want)
		}
	}
}
