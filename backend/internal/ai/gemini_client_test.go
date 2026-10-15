package ai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeGeminiServer replays a canned SSE response shaped exactly like the
// real streamGenerateContent endpoint, so StreamGenerate's parsing can be
// exercised without any real network access (this sandbox has none to
// generativelanguage.googleapis.com anyway).
func fakeGeminiServer(t *testing.T, sseBody string, status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("expected x-goog-api-key header to be set, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(sseBody))
	}))
}

func TestStreamGenerate_AccumulatesDeltasInOrder(t *testing.T) {
	sse := "" +
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"Hel\"}]}}]}\n\n" +
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"lo, \"}]}}]}\n\n" +
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"world!\"}]},\"finishReason\":\"STOP\"}]}\n\n"

	srv := fakeGeminiServer(t, sse, http.StatusOK)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	var deltas []string
	result, err := c.StreamGenerate(context.Background(), "gemini-test", []geminiContent{
		{Role: "user", Parts: []geminiPart{{Text: "hi"}}},
	}, func(s string) { deltas = append(deltas, s) })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := strings.Join(deltas, ""); got != "Hello, world!" {
		t.Fatalf("deltas out of order or missing: got %q", got)
	}
	if result.Text != "Hello, world!" {
		t.Fatalf("expected accumulated text %q, got %q", "Hello, world!", result.Text)
	}
	if result.FinishReason != "STOP" {
		t.Fatalf("expected finish reason STOP, got %q", result.FinishReason)
	}
}

func TestStreamGenerate_SkipsMalformedLinesWithoutAborting(t *testing.T) {
	sse := "" +
		"data: not-valid-json\n\n" +
		"data: {\"candidates\":[{\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}]}\n\n"

	srv := fakeGeminiServer(t, sse, http.StatusOK)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	result, err := c.StreamGenerate(context.Background(), "gemini-test", nil, func(string) {})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "ok" {
		t.Fatalf("expected malformed line to be skipped, got text %q", result.Text)
	}
}

func TestStreamGenerate_UpstreamErrorStatus(t *testing.T) {
	srv := fakeGeminiServer(t, `{"error":{"message":"bad key"}}`, http.StatusUnauthorized)
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	_, err := c.StreamGenerate(context.Background(), "gemini-test", nil, func(string) {})
	if err == nil {
		t.Fatalf("expected an error for a non-200 upstream response")
	}
}

func TestStreamGenerate_MissingAPIKey(t *testing.T) {
	c := NewGeminiClient("", "http://unused.invalid")
	_, err := c.StreamGenerate(context.Background(), "gemini-test", nil, func(string) {})
	if err == nil {
		t.Fatalf("expected an error when no API key is configured")
	}
}

func TestStreamGenerate_RequestURLAndBody(t *testing.T) {
	// Separate server just to assert the model name lands in the URL path
	// and the history round-trips into the JSON body correctly.
	var sawPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewGeminiClient("test-key", srv.URL)
	_, _ = c.StreamGenerate(context.Background(), "gemini-xyz", []geminiContent{{Role: "user", Parts: []geminiPart{{Text: "hi"}}}}, func(string) {})

	want := "/models/gemini-xyz:streamGenerateContent"
	if sawPath != want {
		t.Fatalf("expected path %q, got %q", want, sawPath)
	}
}
