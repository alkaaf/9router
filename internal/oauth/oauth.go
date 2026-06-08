// Package oauth provides the generic OAuth provider/action router for
// /api/oauth/[provider]/[action].
package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// CLITokenHeader / CLITokenSalt match src/dashboardGuard.js.
const (
	CLITokenHeader = "x-9r-cli-token"
	CLITokenSalt   = "9r-cli-auth"
)

// Context is the per-request context passed to every OAuth handler.
type Context struct {
	Ctx      context.Context
	Provider string
	Action   string
	Body     []byte
	CLIToken string
	Query    map[string]string
	AuthOK   bool
}

// HandlerFunc is the signature every provider/action handler must satisfy.
type HandlerFunc func(c *Context) (any, error)

// HandlerError carries a stable error code + human-readable message.
type HandlerError struct {
	Code    string
	Message string
}

func (e *HandlerError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// NewHandlerError builds a HandlerError.
func NewHandlerError(code, message string) *HandlerError {
	return &HandlerError{Code: code, Message: message}
}

// AsHandlerError unwraps an error chain to a *HandlerError, returning nil
// if the chain does not contain one.
func AsHandlerError(err error) *HandlerError {
	if err == nil {
		return nil
	}
	var he *HandlerError
	if errors.As(err, &he) {
		return he
	}
	return nil
}

// Router dispatches OAuth endpoint requests to registered provider/action
// handlers. Safe for concurrent registration at startup; read-only once the
// server is live.
type Router struct {
	mu       sync.RWMutex
	handlers map[string]map[string]HandlerFunc
}

// NewRouter returns an empty Router.
func NewRouter() *Router {
	return &Router{handlers: make(map[string]map[string]HandlerFunc)}
}

// Register adds a handler for (provider, action). Panics on duplicate or
// empty — this is a programming error caught at startup.
func (r *Router) Register(provider, action string, h HandlerFunc) {
	if provider == "" || action == "" {
		panic("oauth: Register requires non-empty provider and action")
	}
	if h == nil {
		panic(fmt.Sprintf("oauth: nil handler for %s/%s", provider, action))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	byAction, ok := r.handlers[provider]
	if !ok {
		byAction = make(map[string]HandlerFunc)
		r.handlers[provider] = byAction
	}
	if _, exists := byAction[action]; exists {
		panic(fmt.Sprintf("oauth: duplicate registration for %s/%s", provider, action))
	}
	byAction[action] = h
}

// Lookup returns the handler registered for (provider, action) and true,
// or nil and false if no handler is registered.
func (r *Router) Lookup(provider, action string) (HandlerFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byAction, ok := r.handlers[provider]
	if !ok {
		return nil, false
	}
	h, ok := byAction[action]
	return h, ok
}

// Dispatch runs the handler for (provider, action) with the supplied
// context. Returns ErrNotFound if no handler is registered.
func (r *Router) Dispatch(c *Context) (any, error) {
	h, ok := r.Lookup(c.Provider, c.Action)
	if !ok {
		return nil, ErrNotFound
	}
	return h(c)
}

// ErrNotFound is returned by Dispatch when the (provider, action) pair is
// not registered.
var ErrNotFound = errors.New("oauth: provider/action not found")

// ---------------------------------------------------------------------------
// CLI token auth (mirrors src/dashboardGuard.js)
// ---------------------------------------------------------------------------

// CLITokenValidator verifies the value of the x-9r-cli-token header.
type CLITokenValidator struct {
	mu        sync.RWMutex
	machineID string
}

// NewCLITokenValidator returns an empty validator.
func NewCLITokenValidator() *CLITokenValidator {
	return &CLITokenValidator{}
}

// SetMachineID updates the machine ID used to derive the canonical token.
func (v *CLITokenValidator) SetMachineID(machineID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.machineID = machineID
}

// MachineID returns the currently configured machine ID.
func (v *CLITokenValidator) MachineID() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.machineID
}

// Token returns the canonical CLI token derived from the current machine
// ID. Empty string if no machine ID is configured.
func (v *CLITokenValidator) Token() string {
	return DeriveCLIToken(v.MachineID())
}

// Validate returns true if the supplied token matches the canonical value.
func (v *CLITokenValidator) Validate(supplied string) bool {
	if supplied == "" {
		return false
	}
	expected := v.Token()
	if expected == "" {
		return false
	}
	return constantTimeEqual(expected, supplied)
}

// DeriveCLIToken hashes machineID with the canonical salt. Matches the
// Node.js getConsistentMachineId(salt) helper.
func DeriveCLIToken(machineID string) string {
	if machineID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(CLITokenSalt + ":" + machineID))
	return hex.EncodeToString(sum[:])
}

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

// ExtractCLITokenHeader returns the value of the x-9r-cli-token header
// (case-insensitive).
func ExtractCLITokenHeader(headers map[string][]string) string {
	for k, vals := range headers {
		if strings.EqualFold(k, CLITokenHeader) && len(vals) > 0 {
			return vals[0]
		}
	}
	return ""
}
