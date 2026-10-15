package metrics

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type hijackRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func TestMiddlewarePreservesHijacker(t *testing.T) {
	var sawHijacker bool
	h := Middleware(func(*http.Request) string { return "/ws" })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		sawHijacker = ok
		if ok {
			_, _, _ = hj.Hijack()
		}
	}))
	rec := &hijackRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/ws", nil))
	if !sawHijacker || !rec.hijacked {
		t.Fatalf("wrapped writer must implement http.Hijacker and delegate (saw=%v hijacked=%v)", sawHijacker, rec.hijacked)
	}
}
