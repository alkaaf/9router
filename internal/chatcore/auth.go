package chatcore

import (
	"net/http"
	"strings"
)

// bearerPrefix is the case-sensitive scheme recognised by
// extractApiKey in the Node.js backend. Only "Bearer " (capital B,
// trailing space) qualifies; any other scheme (Basic, Digest, custom
// tokens, raw keys, etc.) is ignored so that "Authorization: Basic
// abc" does not leak the basic-auth secret as a 9router API key.
const bearerPrefix = "Bearer "

// ExtractAPIKey returns the API key for the incoming chat request,
// matching the priority order of the Node.js
// open-sse/services/auth.js#extractApiKey helper:
//
//  1. Authorization: Bearer <key>  (OpenAI format)
//  2. x-api-key: <key>             (Anthropic format)
//
// If neither header is present, or the Authorization header uses a
// non-Bearer scheme, the function returns an empty string. The
// function is safe to call with a nil header map.
//
// The function deliberately does NOT return (key, ok) — the Node.js
// implementation uses `null` to mean "no key", which downstream code
// checks with `if (apiKey)` and treats identically to an empty
// string. Returning "" keeps that idiom one-to-one.
func ExtractAPIKey(r *http.Request) string {
	if r == nil {
		return ""
	}

	// 1. Authorization: Bearer <key>  — the canonical OpenAI form.
	//    http.Header.Get is case-insensitive (it canonicalises the
	//    key) so "authorization", "Authorization", and "AUTHORIZATION"
	//    all resolve the same. Slice 7 to drop the "Bearer " prefix.
	if v := r.Header.Get("Authorization"); strings.HasPrefix(v, bearerPrefix) {
		return v[len(bearerPrefix):]
	}

	// 2. x-api-key: <key>  — the Anthropic Messages format. We do not
	//    strip whitespace; whatever the client sent is what we hand
	//    to the credential validator. (Anthropic keys never contain
	//    spaces in practice.)
	if v := r.Header.Get("x-api-key"); v != "" {
		return v
	}

	return ""
}
