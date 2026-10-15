package users

import "testing"

func TestUsernamePattern(t *testing.T) {
	cases := []struct {
		username string
		want     bool
	}{
		{"john_doe", true},
		{"a12", true},
		{"abc", true},
		{"ab", false},       // too short
		{"1abc", false},     // must start with a letter
		{"John", false},     // uppercase not allowed
		{"john doe", false}, // space not allowed
		{"john-doe", false}, // hyphen not allowed
		{"", false},
		{"averylongusernamethatistoolongtobevalid", false}, // over 20 chars
	}
	for _, c := range cases {
		if got := usernamePattern.MatchString(c.username); got != c.want {
			t.Errorf("usernamePattern.MatchString(%q) = %v, want %v", c.username, got, c.want)
		}
	}
}
