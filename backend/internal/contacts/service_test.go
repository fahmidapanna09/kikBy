package contacts

import "testing"

func TestE164Pattern(t *testing.T) {
	cases := []struct {
		phone string
		want  bool
	}{
		{"+8801712345678", true},
		{"+14155552671", true},
		{"01712345678", false},
		{"+0123456789", false},
		{"+1", false},
		{"", false},
	}
	for _, c := range cases {
		if got := e164Pattern.MatchString(c.phone); got != c.want {
			t.Errorf("e164Pattern.MatchString(%q) = %v, want %v", c.phone, got, c.want)
		}
	}
}
