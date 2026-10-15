package chat

import (
	"testing"
	"unicode/utf8"
)

func TestDirectKey_OrderIndependent(t *testing.T) {
	a := directKey("user-1", "user-2")
	b := directKey("user-2", "user-1")
	if a != b {
		t.Fatalf("expected order-independent key, got %q vs %q", a, b)
	}
	if a == "" {
		t.Fatal("expected non-empty key")
	}
}

func TestPreviewFor_TextTruncated(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += "a"
	}
	msg := Message{Type: "text", Body: &long}
	preview := previewFor(msg)
	if n := utf8.RuneCountInString(preview); n != 81 { // 80 chars + the ellipsis
		t.Fatalf("expected 81 runes (80 + ellipsis), got %d", n)
	}
}

func TestPreviewFor_NeverSplitsMultibyteCharacters(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "বা" // 2 runes, 6 bytes: a byte-based cut at 80 would land mid-character
	}
	preview := previewFor(Message{Type: "text", Body: &long})
	if !utf8.ValidString(preview) {
		t.Fatalf("preview is not valid UTF-8: %q", preview)
	}
	if n := utf8.RuneCountInString(preview); n != 81 {
		t.Fatalf("expected 81 runes, got %d", n)
	}
}

func TestPreviewFor_NonTextNeverLeaksBody(t *testing.T) {
	body := "some caption that should not appear in the push preview"
	msg := Message{Type: "image", Body: &body}
	preview := previewFor(msg)
	if preview == body {
		t.Fatal("non-text message preview must not be the raw body")
	}
}

func TestEnvelope_HasStableShape(t *testing.T) {
	env := NewEnvelope(EventMessageNew, "conv-1", "device-1", MessageAckPayload{ClientTmpID: "tmp-1", ServerID: "srv-1", Seq: 1})
	if env.Type != EventMessageNew {
		t.Fatalf("expected type %q, got %q", EventMessageNew, env.Type)
	}
	if env.EventID == "" {
		t.Fatal("expected a non-empty event_id")
	}
	if env.Version != 1 {
		t.Fatalf("expected version 1, got %d", env.Version)
	}
}
