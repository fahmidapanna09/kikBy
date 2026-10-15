package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GeminiClient talks to the Gemini "streamGenerateContent" REST endpoint.
// The API key lives only here, read once at startup from an env var (see
// config.go) — it is never logged and never sent to the Android client;
// the client only ever talks to our own backend, which proxies the model.
type GeminiClient struct {
	httpClient *http.Client
	apiKey     string
	baseURL    string // e.g. https://generativelanguage.googleapis.com/v1beta — overridable in tests
}

func NewGeminiClient(apiKey, baseURL string) *GeminiClient {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &GeminiClient{
		httpClient: &http.Client{
			// No overall request timeout here: this is a streaming call and
			// a long model response is expected to take a while. The caller
			// controls cancellation via ctx (e.g. client disconnect).
			Timeout: 0,
		},
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role"` // "user" | "model" — Gemini has no separate "system" role in contents; see service.go
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

type geminiStreamChunk struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *geminiError      `json:"error,omitempty"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// StreamResult is the fully-accumulated outcome of one streamed turn.
type StreamResult struct {
	Text         string
	FinishReason string
}

// StreamGenerate sends the full turn history (already role-mapped to
// "user"/"model") plus calls onDelta(text) for every incremental text
// chunk Gemini sends over the SSE stream, in order, as they arrive — the
// handler forwards each delta straight to the client so the UI can render
// a live-typing effect. It returns once the stream ends (or ctx is
// cancelled), with the full concatenated text and the model's reported
// finish reason.
func (c *GeminiClient) StreamGenerate(ctx context.Context, model string, history []geminiContent, onDelta func(string)) (StreamResult, error) {
	if c.apiKey == "" {
		return StreamResult{}, fmt.Errorf("ai: GEMINI_API_KEY is not configured")
	}

	reqBody, err := json.Marshal(geminiRequest{Contents: history})
	if err != nil {
		return StreamResult{}, err
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", c.baseURL, model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return StreamResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	// x-goog-api-key keeps the key out of the URL (and therefore out of any
	// proxy/access logs that record query strings) — never log req.URL with
	// the key appended as a query param instead.
	req.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return StreamResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return StreamResult{}, fmt.Errorf("%w: gemini returned %d: %s", ErrUpstream, resp.StatusCode, string(body))
	}

	var full strings.Builder
	finishReason := ""

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk geminiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// A malformed/partial line shouldn't abort an otherwise-good
			// stream; skip it and keep reading.
			continue
		}
		if chunk.Error != nil {
			return StreamResult{Text: full.String()}, fmt.Errorf("%w: %s", ErrUpstream, chunk.Error.Message)
		}
		if len(chunk.Candidates) == 0 {
			continue
		}
		cand := chunk.Candidates[0]
		if cand.FinishReason != "" {
			finishReason = cand.FinishReason
		}
		for _, part := range cand.Content.Parts {
			if part.Text == "" {
				continue
			}
			full.WriteString(part.Text)
			onDelta(part.Text)
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return StreamResult{Text: full.String(), FinishReason: finishReason}, fmt.Errorf("%w: reading stream: %v", ErrUpstream, err)
	}

	return StreamResult{Text: full.String(), FinishReason: finishReason}, nil
}
