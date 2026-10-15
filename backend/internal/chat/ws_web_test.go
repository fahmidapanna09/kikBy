package chat

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTokenFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Authorization", "Bearer abc.def")
	if got := tokenFromRequest(r); got != "abc.def" {
		t.Fatalf("header token: %q", got)
	}
	r = httptest.NewRequest("GET", "/ws", nil)
	r.Header.Set("Sec-WebSocket-Protocol", "kikbite.v1, bearer.aaa.bbb.ccc")
	if got := tokenFromRequest(r); got != "aaa.bbb.ccc" {
		t.Fatalf("subprotocol token: %q", got)
	}
	if got := tokenFromRequest(httptest.NewRequest("GET", "/ws", nil)); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestCheckOrigin(t *testing.T) {
	h := &WSHandler{}
	h.SetAllowedOrigins([]string{"https://app.kikbite.com/", "HTTP://localhost:4173"})
	mk := func(origin string) *http.Request {
		r := httptest.NewRequest("GET", "http://api.kikbite.com/ws", nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	cases := map[string]bool{
		"":                         true, // native client
		"https://app.kikbite.com":  true,
		"http://localhost:4173":    true,
		"https://api.kikbite.com":  true, // same host as the request
		"https://evil.example.com": false,
		"http://app.kikbite.com":   false, // scheme matters
		"not a url":                false,
	}
	for origin, want := range cases {
		if got := h.checkOrigin(mk(origin)); got != want {
			t.Errorf("origin %q: got %v want %v", origin, got, want)
		}
	}
}
