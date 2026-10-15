package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaskEmail(t *testing.T) {
	for in, want := range map[string]string{"alex@example.com": "a***@example.com", "x@y.io": "x***@y.io", "nope": "***", "@a.b": "***"} {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSecurityNoticeEmail(t *testing.T) {
	var got resendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"n1"}`))
	}))
	defer srv.Close()
	m := newTestMailer(srv.URL)
	if err := m.SendSecurityNotice(context.Background(), "old@example.com", SecurityNotice{Kind: NoticeEmailChangeRequired, Detail: "n***@new.com"}); err != nil {
		t.Fatal(err)
	}
	if got.To[0] != "old@example.com" || !strings.Contains(got.Text, "n***@new.com") || strings.Contains(got.Text, "http") {
		t.Fatalf("notice must go to the old address, mention the masked new one and carry no link: %+v", got)
	}
	if err := m.SendSecurityNotice(context.Background(), "a@b.co", SecurityNotice{Kind: "unknown"}); err != nil {
		t.Fatal("unknown kinds are ignored, not errors")
	}
}
