package chatcore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Response is the successful return value of an ExecFunc. The
// concrete type lives in the executor package; this is a
// documentation alias.
type Response struct {
	// Status is the upstream HTTP status code (e.g. 200).
	Status int
	// Body is the response payload (may be SSE chunks or a JSON
	// object). The caller decides how to relay it.
	Body []byte
	// Model is the canonical model identifier (e.g.
	// "openai/gpt-4o"). It may differ from the input model when
	// the combo executor advanced to a fallback model.
	Model string
}

// UpstreamError is the canonical error type an ExecFunc returns when
// an upstream call fails. It carries the HTTP status and the error
// text so ComboFallback can decide whether to advance.
type UpstreamError struct {
	Status int
	Text   string
	Cause  error
}

func (e *UpstreamError) Error() string {
	if e.Text != "" {
		return fmt.Sprintf("upstream %d: %s", e.Status, e.Text)
	}
	return fmt.Sprintf("upstream %d", e.Status)
}

func (e *UpstreamError) Unwrap() error {
	return e.Cause
}

// NewUpstreamError builds a *UpstreamError with the given status +
// text. Cause is optional.
func NewUpstreamError(status int, text string, cause error) *UpstreamError {
	return &UpstreamError{Status: status, Text: text, Cause: cause}
}

// ComboFallback executes a list of models in order with fallback support.
//
// For each model in models it calls exec(ctx, model). On success the
// response is returned immediately. On error the fallback decision is
// evaluated: fallbackable errors (rate-limit, 5xx, auth, network)
// cause the loop to continue; non-fallbackable errors (400, 422)
// cause an immediate return.
//
// Transient errors (503) with a small cooldown trigger an optional
// sleep before advancing.
//
// The earliest retry-after across all attempts is tracked via
// EarliestRetryAfter so callers can set Retry-After headers when
// every model has failed.
type ComboFallback struct {
	exec            ExecFunc
	retryAfterMin   *time.Time
	cooldownOn503Ms int
}

// ExecFunc is the per-model callable. The context is cancelled on
// client disconnect.
type ExecFunc func(ctx context.Context, model string) (*Response, error)

// ComboFallbackOption tweaks ComboFallback behaviour.
type ComboFallbackOption func(*ComboFallback)

// WithCooldownOn503 sets the maximum cooldown (ms) to wait after a
// 503 before continuing to the next model. A value of 0 means no
// wait. Default 5000.
func WithCooldownOn503(ms int) ComboFallbackOption {
	return func(c *ComboFallback) { c.cooldownOn503Ms = ms }
}

// NewComboFallback returns a ComboFallback configured with the
// supplied exec function.
func NewComboFallback(exec ExecFunc, opts ...ComboFallbackOption) *ComboFallback {
	cf := &ComboFallback{
		exec:            exec,
		cooldownOn503Ms: 5000,
	}
	for _, o := range opts {
		o(cf)
	}
	return cf
}

// EarliestRetryAfter returns the earliest lock-expiry observed across
// all attempts, or the zero time when none were recorded. The caller
// uses this to set a Retry-After header when every attempt fails.
func (c *ComboFallback) EarliestRetryAfter() time.Time {
	if c.retryAfterMin == nil {
		return time.Time{}
	}
	return *c.retryAfterMin
}

// classifyUpstreamError inspects err and returns (status, text).
// Defaults to (0, err.Error()) when err is not an UpstreamError.
func classifyUpstreamError(err error) (int, string) {
	if err == nil {
		return http.StatusOK, ""
	}
	var ue *UpstreamError
	if errors.As(err, &ue) {
		return ue.Status, ue.Text
	}
	return 0, err.Error()
}

// Execute tries each model in models and returns the first success.
// The context may be cancelled by the caller to abort an in-flight
// upstream request (e.g. on client disconnect).
//
// Returns (*Response, nil) on the first successful attempt,
// (nil, non-nil error) on the last failing attempt, or (nil,
// context.Canceled) if the context was cancelled before any attempt
// completed.
func (c *ComboFallback) Execute(ctx context.Context, models []string) (*Response, error) {
	if c == nil || c.exec == nil {
		return nil, errors.New("combo fallback not configured")
	}
	if len(models) == 0 {
		return nil, errors.New("combo: no models to try")
	}

	var lastErr error
	for i, model := range models {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, err := c.exec(ctx, model)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		status, text := classifyUpstreamError(err)
		decision := CheckFallbackError(status, text, 0)

		if !decision.ShouldFallback {
			return nil, &ComboError{
				Model:     model,
				Attempt:   i + 1,
				Total:     len(models),
				Err:       err,
				Retryable: false,
			}
		}

		if decision.CooldownMs > 0 {
			expiry := time.Now().Add(time.Duration(decision.CooldownMs) * time.Millisecond)
			if c.retryAfterMin == nil || expiry.Before(*c.retryAfterMin) {
				c.retryAfterMin = &expiry
			}
		}

		if c.cooldownOn503Ms > 0 && decision.CooldownMs > 0 && decision.CooldownMs <= c.cooldownOn503Ms {
			select {
			case <-time.After(time.Duration(decision.CooldownMs) * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, &ComboError{
		Model:     models[len(models)-1],
		Attempt:   len(models),
		Total:     len(models),
		Err:       lastErr,
		Retryable: true,
	}
}

// ComboError is the error returned from ComboFallback.Execute when
// every model has failed (or a non-fallbackable error is hit).
type ComboError struct {
	Model     string
	Attempt   int
	Total     int
	Err       error
	Retryable bool
}

func (e *ComboError) Error() string {
	return fmt.Sprintf("combo attempt %d/%d on %s: %v", e.Attempt, e.Total, e.Model, e.Err)
}

func (e *ComboError) Unwrap() error {
	return e.Err
}
