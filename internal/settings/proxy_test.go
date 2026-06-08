package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProxyTestRequest is the request body for POST /api/settings/proxy-test.
type ProxyTestRequest struct {
	ProxyURL  string `json:"proxyUrl"`
	TestURL   string `json:"testUrl"`
	TimeoutMs int    `json:"timeoutMs"`
}

// ProxyTestResponse is the response shape.
type ProxyTestResponse struct {
	OK        bool   `json:"ok"`
	LatencyMs int    `json:"latencyMs,omitempty"`
	Status    int    `json:"status,omitempty"`
	Error     string `json:"error,omitempty"`
}

const (
	defaultTestURL   = "https://api.openai.com/v1/models"
	defaultTimeoutMs = 10000
)

// ProxyTestHandler returns an http.HandlerFunc for POST
// /api/settings/proxy-test.
func ProxyTestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		var req ProxyTestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "Invalid JSON body")
			return
		}
		if strings.TrimSpace(req.ProxyURL) == "" {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", "proxyUrl is required")
			return
		}
		if _, err := url.Parse(req.ProxyURL); err != nil || !isValidProxyURL(req.ProxyURL) {
			writeJSONError(w, http.StatusBadRequest, "BAD_REQUEST", fmt.Sprintf("Invalid proxy URL: %s", req.ProxyURL))
			return
		}

		testURL := req.TestURL
		if testURL == "" {
			testURL = defaultTestURL
		}
		timeoutMs := req.TimeoutMs
		if timeoutMs <= 0 {
			timeoutMs = defaultTimeoutMs
		}

		result := testProxy(req.ProxyURL, testURL, timeoutMs)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

func testProxy(proxyURL, testURL string, timeoutMs int) ProxyTestResponse {
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return ProxyTestResponse{OK: false, Error: fmt.Sprintf("Invalid proxy URL: %v", err)}
	}

	transport := &http.Transport{
		Proxy: http.ProxyURL(proxy),
		DialContext: (&net.Dialer{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		}).DialContext,
		TLSHandshakeTimeout: time.Duration(timeoutMs) * time.Millisecond,
	}
	client := &http.Client{
		Timeout:   time.Duration(timeoutMs) * time.Millisecond,
		Transport: transport,
	}

	start := time.Now()
	resp, err := client.Get(testURL)
	latency := int(time.Since(start).Milliseconds())

	if err != nil {
		return classifyError(err, latency)
	}
	defer resp.Body.Close()

	return ProxyTestResponse{
		OK:        true,
		LatencyMs: latency,
		Status:    resp.StatusCode,
	}
}

func classifyError(err error, latency int) ProxyTestResponse {
	if err == nil {
		return ProxyTestResponse{OK: true, LatencyMs: latency}
	}
	msg := err.Error()
	switch {
	case isTimeoutError(err):
		return ProxyTestResponse{OK: false, Error: "Proxy test timed out", LatencyMs: latency}
	case isConnectionRefused(err):
		return ProxyTestResponse{OK: false, Error: "Connection refused", LatencyMs: latency}
	case isSSLError(err):
		return ProxyTestResponse{OK: false, Error: "SSL error", LatencyMs: latency}
	default:
		return ProxyTestResponse{OK: false, Error: msg, LatencyMs: latency}
	}
}

func isTimeoutError(err error) bool {
	if ne, ok := err.(*url.Error); ok {
		return ne.Timeout()
	}
	return strings.Contains(err.Error(), "timeout")
}

func isConnectionRefused(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Op == "dial" && strings.Contains(opErr.Err.Error(), "refused")
	}
	return strings.Contains(err.Error(), "connection refused")
}

func isSSLError(err error) bool {
	return strings.Contains(err.Error(), "certificate") ||
		strings.Contains(err.Error(), "SSL") ||
		strings.Contains(err.Error(), "TLS")
}

func isValidProxyURL(s string) bool {
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "socks5://") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return u.Host != ""
}
