package executor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// KiroEventFrame is one parsed AWS EventStream binary frame.
type KiroEventFrame struct {
	Headers   map[string]string
	Payload   json.RawMessage
	EventType string
}

// ParseKiroEventFrame reads one binary EventStream frame from r and
// returns its parsed content. Frame format:
//
//	total_length   4 bytes BE
//	headers_length 4 bytes BE
//	headers        header block
//	payload        (total_length - 4 - headers_length) bytes
//	crc32          4 bytes BE (skipped)
//
// The :event-type header maps to the AWS CodeWhisperer event name.
func ParseKiroEventFrame(r io.Reader) (*KiroEventFrame, error) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, fmt.Errorf("read frame header: %w", err)
	}
	totalLen := int(binary.BigEndian.Uint32(hdr[0:4]))
	hdrLen := int(binary.BigEndian.Uint32(hdr[4:8]))
	headers := make(map[string]string)
	if hdrLen > 0 {
		rawHdr := make([]byte, hdrLen)
		if _, err := io.ReadFull(r, rawHdr); err != nil {
			return nil, fmt.Errorf("read headers: %w", err)
		}
		headers, _ = parseEventStreamHeaders(rawHdr)
	}
	payloadLen := totalLen - 4 - hdrLen
	if payloadLen < 0 {
		return nil, fmt.Errorf("invalid frame: total=%d hdr=%d", totalLen, hdrLen)
	}
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, fmt.Errorf("read payload: %w", err)
		}
	}
	return &KiroEventFrame{
		Headers:   headers,
		Payload:   payload,
		EventType: headers[":event-type"],
	}, nil
}

// parseEventStreamHeaders deserializes a binary EventStream header
// block. Format per header:
//
//	name_length    1 byte
//	name           UTF-8
//	value_type    1 byte
//	value_length  2 bytes BE (0x07 = string)
//	value          UTF-8
func parseEventStreamHeaders(raw []byte) (map[string]string, error) {
	out := make(map[string]string)
	for len(raw) >= 1 {
		nameLen := int(raw[0])
		if len(raw) < 1+nameLen+1+2 {
			return nil, errors.New("truncated header")
		}
		name := string(raw[1 : 1+nameLen])
		pos := 1 + nameLen
		pos++ // skip value_type byte
		valLen := int(binary.BigEndian.Uint16(raw[pos : pos+2]))
		pos += 2
		if len(raw) < pos+valLen {
			return nil, errors.New("truncated header value")
		}
		value := string(raw[pos : pos+valLen])
		out[name] = value
		raw = raw[pos+valLen:]
	}
	return out, nil
}

// ────────────────────────────────────────────────────────────────────
// KiroExecutor
// ────────────────────────────────────────────────────────────────────

// KiroExecutor handles Amazon CodeWhisperer's AWS EventStream protocol.
// The executor:
//
//   - Emits the Amz-Sdk-Request and Amz-Sdk-Invocation-Id headers on
//     every request.
//   - Reads binary EventStream frames from the response body.
//   - Maps event types to OpenAI-style SSE chunks (done in the chat
//     handler / translator).
//   - Provides custom retry logic that includes 429 and 408 in
//     addition to the BaseExecutor defaults.
type KiroExecutor struct {
	*BaseExecutor
}

// NewKiroExecutor returns a KiroExecutor.
func NewKiroExecutor() *KiroExecutor {
	cfg := &ProviderConfig{
		Provider:   "kiro",
		BaseURLs:   []string{"https://prod.kiro.amazontrust.com"},
		AuthHeader: "Authorization",
		MaxRetries: DefaultMaxRetries,
	}
	return &KiroExecutor{BaseExecutor: NewBaseExecutor(cfg, nil)}
}

// BuildHeaders adds the AWS SDK headers required by the Kiro endpoint.
func (k *KiroExecutor) BuildHeaders(req *Request, creds *Credentials) http.Header {
	h := k.BaseExecutor.BuildHeaders(req, creds)
	h.Set("Amz-Sdk-Request", "attempt=1")
	h.Set("Amz-Sdk-Invocation-Id", generateID())
	return h
}

// TransformRequest is a passthrough — the upstream translator produces
// the Kiro request shape.
func (k *KiroExecutor) TransformRequest(model string, body []byte, stream bool, creds *Credentials) ([]byte, error) {
	return body, nil
}

// Execute runs the Kiro request with custom retry logic. The
// BaseExecutor default policy is augmented to also retry on 429 and
// 408 (request timeout) which the Node.js implementation treats as
// transient for Kiro.
func (k *KiroExecutor) Execute(ctx context.Context, req *Request, creds *Credentials) (*Response, error) {
	if k.NeedsRefresh(creds) {
		var err error
		creds, err = k.RefreshCredentials(ctx, creds)
		if err != nil {
			return nil, &ExecutorError{Code: CodeAuthFailed, Message: "token refresh: " + err.Error()}
		}
	}
	body, err := k.TransformRequest(req.Model, req.Body, req.Stream, creds)
	if err != nil {
		return nil, &ExecutorError{Code: CodeBadRequest, Message: "transform: " + err.Error()}
	}

	kiroShouldRetry := func(status int) bool {
		switch status {
		case 429, http.StatusRequestTimeout:
			return true
		default:
			return k.ShouldRetry(status, 0)
		}
	}

	maxRetries := k.MaxRetriesFor()
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, &ExecutorError{Code: CodeTimeout, Message: "context deadline exceeded"}
			}
			return nil, &ExecutorError{Code: CodeCanceled, Message: "context canceled"}
		}
		if attempt > 0 {
			delay := backoffDelay(attempt)
			select {
			case <-ctx.Done():
				return nil, &ExecutorError{Code: CodeCanceled, Message: "context canceled during backoff"}
			case <-time.After(delay):
			}
		}
		httpReq, err := k.buildHTTPRequest(ctx, req, body, creds, 0)
		if err != nil {
			return nil, &ExecutorError{Code: CodeBadRequest, Message: err.Error()}
		}
		resp, err := k.client.Do(httpReq)
		if err != nil {
			if attempt == maxRetries {
				return nil, &ExecutorError{Code: CodeUnavailable, Message: err.Error()}
			}
			continue
		}
		if !kiroShouldRetry(resp.StatusCode) {
			return k.toExecutorResponse(resp, 0, attempt+1), nil
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if attempt == maxRetries {
			return nil, k.ParseError(resp.StatusCode, "")
		}
	}
	return nil, &ExecutorError{Code: CodeUnavailable, Message: "all retries exhausted"}
}

// NeedsRefresh returns true when the supplied token is empty.
func (k *KiroExecutor) NeedsRefresh(creds *Credentials) bool {
	if creds == nil || creds.AccessToken == "" {
		return true
	}
	if creds.ExpiresAt == 0 {
		return true // Kiro tokens always have an expiry; missing one is suspicious.
	}
	expiry := time.Unix(creds.ExpiresAt, 0)
	return time.Until(expiry) <= RefreshSkew
}

// RefreshCredentials is a no-op base. The host application's OAuth
// service performs the actual Kiro refresh and provides the new token
// via Credentials.
func (k *KiroExecutor) RefreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
	return creds, nil
}

// EstimateOutputTokens is a heuristic token counter: the Node.js
// implementation uses content_length / 4 for the output token
// estimate.
func EstimateOutputTokens(contentLength int) int {
	return contentLength / 4
}

// EstimateInputTokens returns the input token estimate based on the
// context usage percentage (0..1).
func EstimateInputTokens(contextUsage float64) int {
	return int(contextUsage * 200000)
}

// ScanEventStream reads a stream of AWS EventStream binary frames
// from r and yields parsed KiroEventFrame values. The channel closes
// when the upstream ends, the context is canceled, or a frame parse
// error occurs.
func ScanEventStream(ctx context.Context, r io.Reader) <-chan *KiroEventFrame {
	out := make(chan *KiroEventFrame, 8)
	go func() {
		defer close(out)
		for {
			if err := ctx.Err(); err != nil {
				return
			}
			frame, err := ParseKiroEventFrame(r)
			if err != nil {
				if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
					return
				}
				return
			}
			if frame == nil {
				return
			}
			out <- frame
		}
	}()
	return out
}

func init() {
	Register("kiro", func() Executor { return NewKiroExecutor() })
}

// compiled-in: keep bufio/bytes imported for future use.
var _ = bufio.NewScanner
var _ = bytes.NewReader
