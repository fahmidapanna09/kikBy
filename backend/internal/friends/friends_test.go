package friends

import (
	"errors"
	"testing"
)

func TestCheckID(t *testing.T) {
	const a = "11111111-1111-1111-1111-111111111111"
	const b = "22222222-2222-2222-2222-222222222222"
	if err := checkID(a, b); err != nil {
		t.Errorf("valid pair rejected: %v", err)
	}
	if err := checkID(a, a); !errors.Is(err, ErrSelf) {
		t.Errorf("self not rejected: %v", err)
	}
	for _, bad := range []string{"", "abc", "1; DROP TABLE users", "11111111-1111-1111-1111-11111111111"} {
		if err := checkID(a, bad); !errors.Is(err, ErrInvalidID) {
			t.Errorf("checkID(%q) = %v, want ErrInvalidID", bad, err)
		}
	}
}
