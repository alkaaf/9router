package chatcore

import (
	"encoding/json"
	"net/http"
)

// ErrorBody mirrors the OpenAI error envelope used by
// open-sse/utils/error.js — the Go rewrite must produce a
// byte-for-byte identical response so existing frontend code (and
// third-party tools) that pattern-matches on the shape keeps working.
//
// The shape is:
//
//	{ "error": { "message": "...", "type": "...", "code": "..." } }
type ErrorBody struct {
	Error ErrorInfo `json:"error"`
}

// ErrorInfo is the inner object of an OpenAI-style error response.
type ErrorInfo struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// errorTypesByStatus mirrors the ERROR_TYPES map in
// open-sse/config/errorConfig.js. Status codes that fall outside
// this map default to "server_error" / "internal_server_error" for
// 5xx and "invalid_request_error" / "" for 4xx, exactly like the JS
// implementation.
var errorTypesByStatus = map[int]ErrorInfo{
	http.StatusBadRequest:          {Type: "invalid_request_error", Code: ""},
	http.StatusUnauthorized:        {Type: "invalid_request_error", Code: "invalid_api_key"},
	http.StatusPaymentRequired:     {Type: "invalid_request_error", Code: "insufficient_quota"},
	http.StatusForbidden:           {Type: "invalid_request_error", Code: ""},
	http.StatusNotFound:            {Type: "invalid_request_error", Code: "model_not_found"},
	http.StatusRequestTimeout:      {Type: "invalid_request_error", Code: "request_timeout"},
	http.StatusUnprocessableEntity: {Type: "invalid_request_error", Code: ""},
	http.StatusTooManyRequests:     {Type: "server_error", Code: "rate_limit_reached"},
	http.StatusInternalServerError: {Type: "server_error", Code: "internal_server_error"},
	http.StatusBadGateway:          {Type: "server_error", Code: ""},
	http.StatusServiceUnavailable:  {Type: "server_error", Code: ""},
}

// WriteError serialises an OpenAI-style error envelope to w with the
// given status and message. The Content-Type and CORS headers mirror
// what open-sse/utils/error.js#errorResponse sets so the response is
// drop-in compatible with the Node.js backend.
func WriteError(w http.ResponseWriter, status int, message string) {
	info, ok := errorTypesByStatus[status]
	if !ok {
		if status >= 500 {
			info = ErrorInfo{Type: "server_error", Code: "internal_server_error"}
		} else {
			info = ErrorInfo{Type: "invalid_request_error", Code: ""}
		}
	}

	body := ErrorBody{Error: ErrorInfo{
		Message: message,
		Type:    info.Type,
		Code:    info.Code,
	}}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
