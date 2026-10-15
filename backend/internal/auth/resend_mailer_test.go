package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestMailer(url string) *ResendMailer {
	m := NewResendMailer("re_test_key", "KikBite <noreply@example.com>")
	m.BaseURL = url
	m.retryWait = time.Millisecond
	return m
}

func TestResendSendsExpectedRequest(t *testing.T) {
	var got resendRequest
	var hdr http.Header
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr, path = r.Header.Clone(), r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer srv.Close()

	code := "042917"
	if err := newTestMailer(srv.URL).SendEmailVerification(context.Background(), "bob@example.com", code); err != nil {
		t.Fatal(err)
	}
	if path != "/emails" || hdr.Get("Authorization") != "Bearer re_test_key" || hdr.Get("Content-Type") != "application/json" {
		t.Fatalf("bad request: path=%s hdr=%v", path, hdr)
	}
	if !strings.HasPrefix(hdr.Get("Idempotency-Key"), "kikbite-verify-") {
		t.Fatalf("idempotency key: %q", hdr.Get("Idempotency-Key"))
	}
	if got.From != "KikBite <noreply@example.com>" || len(got.To) != 1 || got.To[0] != "bob@example.com" || got.Subject == "" {
		t.Fatalf("body: %+v", got)
	}
	if !strings.Contains(got.Text, code) || !strings.Contains(got.Text, "10 minutes") {
		t.Fatalf("plain-text part must contain the code and its 10-minute lifetime: %q", got.Text)
	}
	if !strings.Contains(got.HTML, ">"+code+"<") || !strings.Contains(got.HTML, "10 minutes") {
		t.Fatal("html part must show the code in its own box and mention the 10-minute lifetime")
	}
	if strings.Contains(got.Subject, code) {
		t.Fatal("the code must not be in the subject line")
	}
}

func TestResendIdempotencyKeyIsStablePerMailAndDiffersAcrossMails(t *testing.T) {
	a := idempotencyKey("reset", "a@b.co", "https://x/1")
	if a != idempotencyKey("reset", "a@b.co", "https://x/1") {
		t.Fatal("not stable")
	}
	if a == idempotencyKey("reset", "a@b.co", "https://x/2") || a == idempotencyKey("verify", "a@b.co", "https://x/1") {
		t.Fatal("collision")
	}
	if len(a) > 256 {
		t.Fatal("too long for Resend")
	}
}

func TestResendRetriesTransientThenSucceeds(t *testing.T) {
	var calls int32
	var keys []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	if err := newTestMailer(srv.URL).SendPasswordReset(context.Background(), "a@b.co", "https://x/r?token=1"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || keys[0] != keys[1] {
		t.Fatalf("calls=%d keys=%v (retry must reuse the same idempotency key)", calls, keys)
	}
}

func TestResendDoesNotRetryClientErrors(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"name":"validation_error","message":"The kikbite.com domain is not verified."}`))
	}))
	defer srv.Close()
	err := newTestMailer(srv.URL).SendPasswordReset(context.Background(), "a@b.co", "https://x/r?token=1")
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "domain is not verified") {
		t.Fatalf("error should carry status + provider message: %v", err)
	}
	if strings.Contains(err.Error(), "re_test_key") {
		t.Fatal("API key leaked into error")
	}
}

func TestResendNotConfigured(t *testing.T) {
	if err := NewResendMailer("", "").SendPasswordReset(context.Background(), "a@b.co", "x"); err != ErrMailerNotConfigured {
		t.Fatalf("got %v", err)
	}
}

func TestRenderEmailEscapesEveryField(t *testing.T) {
	h := renderEmail(emailSpec{Title: "<b>t</b>", Intro: "<script>x</script>", Code: "<i>1</i>", Link: "https://x/?a=1&b=<script>", Button: "<u>go</u>", Footer: "<img src=x>", ToEmail: "a@b.co<script>"})
	for _, bad := range []string{"<script>", "<b>t</b>", "<i>1</i>", "<img src=x>", "<u>go</u>"} {
		if strings.Contains(h, bad) {
			t.Fatalf("unescaped %q in rendered e-mail", bad)
		}
	}
}

func TestVerifyCodeHelpers(t *testing.T) {
	for i := 0; i < 200; i++ {
		c, err := newVerifyCode()
		if err != nil || len(c) != 6 || normalizeVerifyCode(c) != c {
			t.Fatalf("bad code %q err=%v", c, err)
		}
	}
	for in, want := range map[string]string{"123456": "123456", " 123 456 ": "123456", "123-456": "123456", "12345": "", "1234567": "", "12a456": "", "": "", "١٢٣٤٥٦": ""} {
		if got := normalizeVerifyCode(in); got != want {
			t.Fatalf("normalizeVerifyCode(%q) = %q, want %q", in, got, want)
		}
	}
	if verifyCodeTTL != 10*time.Minute {
		t.Fatalf("codes must live 10 minutes, got %v", verifyCodeTTL)
	}
}
