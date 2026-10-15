package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareRecordsRequestAndStatus(t *testing.T) {
	// Reset package-level metrics so this test doesn't depend on ordering
	// relative to other tests in this package.
	httpRequestsTotal = newCounterVec("http_requests_total", "test", "method", "route", "status")
	httpRequestDuration = newHistogramVec("http_request_duration_seconds", "test",
		[]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}, "method", "route")

	handler := Middleware(func(r *http.Request) string { return "/v1/users/{id}" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/v1/users/123", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 passed through, got %d", rec.Code)
	}

	var buf strings.Builder
	httpRequestsTotal.writeTo(&buf)
	out := buf.String()
	if !strings.Contains(out, `method="POST"`) || !strings.Contains(out, `route="/v1/users/{id}"`) || !strings.Contains(out, `status="201"`) {
		t.Fatalf("expected counter labeled by method/route/status, got:\n%s", out)
	}

	buf.Reset()
	httpRequestDuration.writeTo(&buf)
	out = buf.String()
	if !strings.Contains(out, "http_request_duration_seconds_count") {
		t.Fatalf("expected a duration histogram observation, got:\n%s", out)
	}
}

func TestMiddlewareFallsBackToUnmatchedRoute(t *testing.T) {
	httpRequestsTotal = newCounterVec("http_requests_total", "test", "method", "route", "status")
	httpRequestDuration = newHistogramVec("http_request_duration_seconds", "test", []float64{0.1}, "method", "route")

	handler := Middleware(func(r *http.Request) string { return "" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
	)
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var buf strings.Builder
	httpRequestsTotal.writeTo(&buf)
	if !strings.Contains(buf.String(), `route="unmatched"`) {
		t.Fatalf("expected an empty route pattern to fall back to \"unmatched\", got:\n%s", buf.String())
	}
}

func TestStatusCapturingWriterDefaultsTo200WhenWriteHeaderNeverCalled(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusCapturingWriter{ResponseWriter: rec, status: 200}
	_, _ = sw.Write([]byte("ok")) // handler that never calls WriteHeader explicitly
	if sw.status != 200 {
		t.Fatalf("expected default status 200, got %d", sw.status)
	}
}

func TestHistogramBucketPlacementAndInfBucket(t *testing.T) {
	h := newHistogramVec("test_hist", "test", []float64{0.1, 1}, "x")
	h.Observe(0.05, "a") // bucket 0
	h.Observe(0.5, "a")  // bucket 1
	h.Observe(5, "a")    // +Inf bucket

	var buf strings.Builder
	h.writeTo(&buf)
	out := buf.String()

	// Cumulative counts: le=0.1 -> 1, le=1 -> 2, le=+Inf -> 3
	if !strings.Contains(out, `le="0.1"} 1`) {
		t.Fatalf("expected cumulative count 1 at le=0.1, got:\n%s", out)
	}
	if !strings.Contains(out, `le="1"} 2`) {
		t.Fatalf("expected cumulative count 2 at le=1, got:\n%s", out)
	}
	if !strings.Contains(out, `le="+Inf"} 3`) {
		t.Fatalf("expected cumulative count 3 at le=+Inf, got:\n%s", out)
	}
	if !strings.Contains(out, "test_hist_count{x=\"a\"} 3") {
		t.Fatalf("expected total count 3, got:\n%s", out)
	}
}
