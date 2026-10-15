// Package metrics is a minimal, dependency-free Prometheus-compatible
// metrics recorder and exposition-format writer.
//
// Known gap (disclosed, not hidden): a real deployment should use the
// official github.com/prometheus/client_golang library instead of this
// hand-rolled implementation — it has battle-tested histogram bucketing,
// a pull-based collector model, and much wider ecosystem support (e.g.
// client-side aggregation helpers, exemplars, native histograms). This
// package exists only because the sandbox this project was built in has
// no network egress to proxy.golang.org or to raw github.com module
// downloads, so a brand-new go.mod dependency could not be fetched or
// verified here. The text format this package writes is the real
// Prometheus exposition format (https://prometheus.io/docs/instrumenting/exposition_formats/)
// and is wire-compatible with a real Prometheus server's scraper — only
// the server-side bookkeeping is simplified. Swapping this package out
// for promhttp.Handler() + promauto collectors, with no change to the
// route wiring in cmd/api/main.go, is a drop-in upgrade once this project
// is built somewhere with normal module access.
package metrics

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// counterVec is a counter metric split out by a fixed set of label names.
type counterVec struct {
	name, help string
	labelNames []string

	mu     sync.Mutex
	values map[string]float64 // key: labels joined by \xff, in labelNames order
}

func newCounterVec(name, help string, labelNames ...string) *counterVec {
	return &counterVec{name: name, help: help, labelNames: labelNames, values: map[string]float64{}}
}

func labelKey(labelValues []string) string { return strings.Join(labelValues, "\xff") }

func (c *counterVec) Inc(labelValues ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.values[labelKey(labelValues)]++
}

func (c *counterVec) writeTo(w io.Writer) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.values) == 0 {
		return
	}
	fmt.Fprintf(w, "# HELP %s %s\n", c.name, c.help)
	fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
	for _, k := range sortedKeys(c.values) {
		fmt.Fprintf(w, "%s%s %s\n", c.name, labelsString(c.labelNames, k), formatFloat(c.values[k]))
	}
}

// histogramVec is a histogram metric split out by a fixed set of label
// names, with fixed (non-configurable) bucket boundaries — adequate for
// HTTP request-duration tracking, which is the only histogram this
// project needs.
type histogramVec struct {
	name, help string
	labelNames []string
	buckets    []float64 // ascending, finite boundaries; +Inf bucket is implicit

	mu           sync.Mutex
	bucketCounts map[string][]uint64 // key -> per-bucket count (non-cumulative), len = len(buckets)+1 (last = +Inf)
	sums         map[string]float64
	counts       map[string]uint64
}

func newHistogramVec(name, help string, buckets []float64, labelNames ...string) *histogramVec {
	return &histogramVec{
		name: name, help: help, labelNames: labelNames, buckets: buckets,
		bucketCounts: map[string][]uint64{}, sums: map[string]float64{}, counts: map[string]uint64{},
	}
}

func (h *histogramVec) Observe(value float64, labelValues ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := labelKey(labelValues)
	counts, ok := h.bucketCounts[key]
	if !ok {
		counts = make([]uint64, len(h.buckets)+1)
		h.bucketCounts[key] = counts
	}
	placed := false
	for i, le := range h.buckets {
		if value <= le {
			counts[i]++
			placed = true
			break
		}
	}
	if !placed {
		counts[len(h.buckets)]++ // +Inf bucket
	}
	h.sums[key] += value
	h.counts[key]++
}

func (h *histogramVec) writeTo(w io.Writer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.counts) == 0 {
		return
	}
	fmt.Fprintf(w, "# HELP %s %s\n", h.name, h.help)
	fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)
	for _, k := range sortedKeys(h.counts) {
		counts := h.bucketCounts[k]
		var cumulative uint64
		for i, le := range h.buckets {
			cumulative += counts[i]
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, labelsStringWithExtra(h.labelNames, k, "le", formatFloat(le)), cumulative)
		}
		cumulative += counts[len(h.buckets)]
		fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, labelsStringWithExtra(h.labelNames, k, "le", "+Inf"), cumulative)
		fmt.Fprintf(w, "%s_sum%s %s\n", h.name, labelsString(h.labelNames, k), formatFloat(h.sums[k]))
		fmt.Fprintf(w, "%s_count%s %d\n", h.name, labelsString(h.labelNames, k), h.counts[k])
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func labelsString(labelNames []string, key string) string {
	return labelsStringWithExtra(labelNames, key, "", "")
}

// labelsStringWithExtra renders the Prometheus `{name="value",...}` suffix.
// extraName/extraValue, when extraName is non-empty, are appended last —
// used for the histogram "le" bucket-boundary label.
func labelsStringWithExtra(labelNames []string, key, extraName, extraValue string) string {
	values := strings.Split(key, "\xff")
	parts := make([]string, 0, len(labelNames)+1)
	for i, name := range labelNames {
		v := ""
		if i < len(values) {
			v = values[i]
		}
		parts = append(parts, fmt.Sprintf(`%s=%q`, name, v))
	}
	if extraName != "" {
		parts = append(parts, fmt.Sprintf(`%s=%q`, extraName, extraValue))
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func formatFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", f), "0"), ".")
}

// --- the fixed set of metrics this project exposes ---

var (
	httpRequestsTotal = newCounterVec(
		"http_requests_total", "Total number of HTTP requests handled, by method, route and status code.",
		"method", "route", "status",
	)
	httpRequestDuration = newHistogramVec(
		"http_request_duration_seconds", "HTTP request latency in seconds, by method and route.",
		[]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		"method", "route",
	)
	processStart = time.Now()
)

// RoutePattern abstracts chi's RouteContext lookup so this package doesn't
// have to import chi — callers (cmd/api/main.go) pass the already-resolved
// pattern in. Kept this way so metrics stays usable from any router.
type RoutePattern func(r *http.Request) string

// Middleware records one httpRequestsTotal increment and one
// httpRequestDuration observation per request. routePattern should return
// the low-cardinality route template (e.g. "/v1/users/{id}"), never the
// raw URL path, to avoid unbounded label cardinality from path parameters.
func Middleware(routePattern RoutePattern) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusCapturingWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(sw, r)

			route := routePattern(r)
			if route == "" {
				route = "unmatched"
			}
			status := fmt.Sprintf("%d", sw.status)
			httpRequestsTotal.Inc(r.Method, route, status)
			httpRequestDuration.Observe(time.Since(start).Seconds(), r.Method, route)
		})
	}
}

type statusCapturingWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusCapturingWriter) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusCapturingWriter) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Hijack lets WebSocket upgrades (gorilla/websocket asserts http.Hijacker on the
// writer it is given) pass through this wrapper. Without it /ws fails with
// "response does not implement http.Hijacker".
func (s *statusCapturingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("metrics: underlying ResponseWriter does not support hijacking")
	}
	if !s.wroteHeader {
		s.status = http.StatusSwitchingProtocols
		s.wroteHeader = true
	}
	return h.Hijack()
}

// Flush keeps streaming responses working through the wrapper.
func (s *statusCapturingWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (s *statusCapturingWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Handler serves the current metric values in Prometheus text exposition
// format. Mount it on its own port (see cfg.Metrics.Port in cmd/api/main.go)
// — never on the public API port, so /metrics is never reachable from the
// internet by accident.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprintf(w, "# HELP process_uptime_seconds Time since the process started.\n")
		fmt.Fprintf(w, "# TYPE process_uptime_seconds gauge\n")
		fmt.Fprintf(w, "process_uptime_seconds %s\n", formatFloat(time.Since(processStart).Seconds()))
		httpRequestsTotal.writeTo(w)
		httpRequestDuration.writeTo(w)
	})
}
