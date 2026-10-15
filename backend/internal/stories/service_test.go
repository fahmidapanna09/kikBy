package stories

import (
	"strings"
	"testing"
)

func TestNormalizeCreate_TextStory(t *testing.T) {
	req := CreateStoryRequest{Type: TypeText, TextContent: "  hello  "}
	if err := normalizeCreate(&req, "author-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.TextContent != "hello" {
		t.Fatalf("expected trimmed text, got %q", req.TextContent)
	}
	if req.BackgroundColor != defaultBgColor {
		t.Fatalf("expected default background color, got %q", req.BackgroundColor)
	}
	if req.Privacy != PrivacyContacts {
		t.Fatalf("expected default privacy contacts, got %q", req.Privacy)
	}
}

func TestNormalizeCreate_TextStoryRequiresContent(t *testing.T) {
	req := CreateStoryRequest{Type: TypeText}
	if err := normalizeCreate(&req, "author-1"); err == nil {
		t.Fatal("expected an error for empty text content")
	}
}

func TestNormalizeCreate_TextStoryRejectsMedia(t *testing.T) {
	req := CreateStoryRequest{Type: TypeText, TextContent: "hi", MediaID: "m1"}
	if err := normalizeCreate(&req, "author-1"); err == nil {
		t.Fatal("expected an error: text stories can't carry media")
	}
}

func TestNormalizeCreate_ImageStoryRequiresMedia(t *testing.T) {
	req := CreateStoryRequest{Type: TypeImage}
	if err := normalizeCreate(&req, "author-1"); err == nil {
		t.Fatal("expected an error for a media story with no media_id")
	}
}

func TestNormalizeCreate_InvalidBackgroundColor(t *testing.T) {
	req := CreateStoryRequest{Type: TypeText, TextContent: "hi", BackgroundColor: "teal"}
	if err := normalizeCreate(&req, "author-1"); err == nil {
		t.Fatal("expected an error for a non-hex background color")
	}
}

func TestNormalizeCreate_SelectedPrivacyRequiresAudience(t *testing.T) {
	req := CreateStoryRequest{Type: TypeText, TextContent: "hi", Privacy: PrivacySelected}
	if err := normalizeCreate(&req, "author-1"); err == nil {
		t.Fatal("expected an error: privacy=selected with no audience")
	}
}

func TestNormalizeCreate_ContactsPrivacyIgnoresAudienceList(t *testing.T) {
	req := CreateStoryRequest{Type: TypeText, TextContent: "hi", Privacy: PrivacyContacts, AudienceUserIDs: []string{"u1", "u2"}}
	if err := normalizeCreate(&req, "author-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.AudienceUserIDs) != 0 {
		t.Fatalf("expected audience to be cleared for privacy=contacts, got %v", req.AudienceUserIDs)
	}
}

func TestUniqueWithout_DropsDuplicatesAndExcluded(t *testing.T) {
	got := uniqueWithout([]string{"a", "b", "a", "self", "c"}, "self")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestVisibleSQL_SubstitutesViewerPlaceholder(t *testing.T) {
	out := visibleSQL("$1")
	if !strings.Contains(out, "s.author_id <> $1") {
		t.Fatalf("expected {viewer} substituted with $1, got: %s", out)
	}
	if strings.Contains(out, "{viewer}") {
		t.Fatal("placeholder was not fully substituted")
	}
}
