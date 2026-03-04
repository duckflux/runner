package participant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/duckflux/runner/internal/model"
)

// HTTPParticipant executes an HTTP request built from the participant
// definition. It respects context cancellation/timeouts passed to Execute.
type HTTPParticipant struct {
	url     string
	method  string
	headers map[string]string
	body    any

	client *http.Client
}

// NewHTTP constructs an HTTPParticipant from a participant definition.
// An optional *http.Client may be supplied for testing; if nil the default
// client is used.
func NewHTTP(def model.Participant, client *http.Client) *HTTPParticipant {
	method := strings.ToUpper(def.Method)
	if method == "" {
		method = http.MethodGet
	}
	c := client
	if c == nil {
		c = http.DefaultClient
	}
	return &HTTPParticipant{
		url:     def.URL,
		method:  method,
		headers: def.Headers,
		body:    def.Body,
		client:  c,
	}
}

// Execute builds and sends the configured HTTP request. If the participant
// definition has no body, input is used as the request body instead. Strings
// are written verbatim; all other values are JSON-marshalled. The raw response
// body is returned as a string on success. Non-2xx status codes are treated as
// errors.
func (h *HTTPParticipant) Execute(ctx context.Context, input any) (any, error) {
	if h.url == "" {
		return nil, fmt.Errorf("http participant: url is required")
	}

	// Determine the effective body: definition body takes precedence over input.
	effectiveBody := h.body
	if effectiveBody == nil {
		effectiveBody = input
	}

	var bodyReader io.Reader
	if effectiveBody != nil {
		data, err := inputToBytes(effectiveBody)
		if err != nil {
			return nil, fmt.Errorf("http participant: preparing request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, h.method, h.url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http participant: building request: %w", err)
	}

	// Apply configured headers.
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	// Default Content-Type for requests with a body when not explicitly set.
	if bodyReader != nil && req.Header.Get("Content-Type") == "" {
		if _, isString := effectiveBody.(string); isString {
			req.Header.Set("Content-Type", "text/plain")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http participant: executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http participant: reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http participant: unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Attempt JSON auto-detection: if the response parses as JSON return the
	// decoded value so callers can access fields directly (e.g. step.output.id).
	var decoded any
	if err := json.Unmarshal(respBody, &decoded); err == nil {
		return decoded, nil
	}

	return string(respBody), nil
}
