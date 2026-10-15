package channels

import (
	"errors"
	"testing"
)

func TestErrNotAdmin_IsErrPermissionDenied(t *testing.T) {
	// ErrNotAdmin is an alias kept for readability at call sites; make sure
	// it's never allowed to drift into a second, distinct error value that
	// errors.Is checks elsewhere would silently stop matching.
	if !errors.Is(ErrNotAdmin, ErrPermissionDenied) {
		t.Fatal("ErrNotAdmin must be the same error value as ErrPermissionDenied")
	}
}

func TestPollOptionBounds(t *testing.T) {
	if minPollOptions != 2 {
		t.Errorf("expected minPollOptions = 2, got %d", minPollOptions)
	}
	if maxPollOptions != 10 {
		t.Errorf("expected maxPollOptions = 10, got %d", maxPollOptions)
	}
	if minPollOptions >= maxPollOptions {
		t.Fatal("minPollOptions must be less than maxPollOptions")
	}
}

func TestPostTypeConstants_AreDistinct(t *testing.T) {
	types := []string{PostText, PostImage, PostVideo, PostAudio, PostSticker, PostPoll, PostLink}
	seen := map[string]bool{}
	for _, ty := range types {
		if seen[ty] {
			t.Fatalf("duplicate post type constant: %q", ty)
		}
		seen[ty] = true
	}
}
