package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fakeStore is an in-memory SettingsStore for unit tests. All methods
// use value receivers so tests can pass `store` directly. State that
// must persist across calls (writes, call counts) lives on the shared
// `*fakeStoreState` pointer.
type fakeStore struct {
	st *fakeStoreState
}

type fakeStoreState struct {
	data         map[string]any
	err          error
	updated      map[string]any
	updateErr    error
	updateCalled int
}

func (f fakeStore) GetSettings() (map[string]any, error) {
	if f.st == nil {
		return map[string]any{}, nil
	}
	if f.st.err != nil {
		return nil, f.st.err
	}
	out := make(map[string]any, len(f.st.data))
	for k, v := range f.st.data {
		out[k] = v
	}
	return out, nil
}

func (f fakeStore) UpdateSettings(updates map[string]any) (map[string]any, error) {
	if f.st == nil {
		return updates, nil
	}
	f.st.updateCalled++
	if f.st.updateErr != nil {
		return nil, f.st.updateErr
	}
	if f.st.updated == nil {
		f.st.updated = map[string]any{}
	}
	for k, v := range updates {
		if v == nil {
			delete(f.st.updated, k)
			delete(f.st.data, k)
		} else {
			f.st.updated[k] = v
		}
	}
	merged := map[string]any{}
	for k, v := range f.st.data {
		merged[k] = v
	}
	for k, v := range f.st.updated {
		merged[k] = v
	}
	f.st.data = merged
	return merged, nil
}

func newGetRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/settings", nil)
}

func TestSettingsGet_HappyPath(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "false")
	t.Setenv("ENABLE_TRANSLATOR", "false")
	store := fakeStore{st: &fakeStoreState{data: map[string]any{
		"requireLogin":         true,
		"outboundProxyEnabled": false,
		"outboundProxyUrl":     "",
		"comboStrategy":        "round-robin",
	}}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["requireLogin"] != true {
		t.Errorf("requireLogin = %v, want true", body["requireLogin"])
	}
	if body["comboStrategy"] != "round-robin" {
		t.Errorf("comboStrategy = %v, want round-robin", body["comboStrategy"])
	}
	if _, ok := body["password"]; ok {
		t.Errorf("password should not be present in response")
	}
	if _, ok := body["oidcClientSecret"]; ok {
		t.Errorf("oidcClientSecret should not be present in response")
	}
	if body["hasPassword"] != false {
		t.Errorf("hasPassword = %v, want false", body["hasPassword"])
	}
	if body["oidcConfigured"] != false {
		t.Errorf("oidcConfigured = %v, want false", body["oidcConfigured"])
	}
	if body["enableRequestLogs"] != false {
		t.Errorf("enableRequestLogs = %v, want false", body["enableRequestLogs"])
	}
	if body["enableTranslator"] != false {
		t.Errorf("enableTranslator = %v, want false", body["enableTranslator"])
	}
}

func TestSettingsGet_OIDCConfigured(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "false")
	t.Setenv("ENABLE_TRANSLATOR", "false")
	store := fakeStore{st: &fakeStoreState{data: map[string]any{
		"oidcIssuerUrl":    "https://idp.example.com",
		"oidcClientId":     "client-1",
		"oidcClientSecret": "super-secret",
	}}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["oidcConfigured"] != true {
		t.Errorf("oidcConfigured = %v, want true", body["oidcConfigured"])
	}
	if _, ok := body["oidcClientSecret"]; ok {
		t.Errorf("oidcClientSecret should be redacted")
	}
}

func TestSettingsGet_OIDCMissingField(t *testing.T) {
	cases := []map[string]any{
		{"oidcIssuerUrl": "", "oidcClientId": "c", "oidcClientSecret": "s"},
		{"oidcIssuerUrl": "i", "oidcClientId": "", "oidcClientSecret": "s"},
		{"oidcIssuerUrl": "i", "oidcClientId": "c", "oidcClientSecret": ""},
	}
	for i, raw := range cases {
		t.Setenv("ENABLE_REQUEST_LOGS", "false")
		t.Setenv("ENABLE_TRANSLATOR", "false")
		store := fakeStore{st: &fakeStoreState{data: raw}}
		rec := httptest.NewRecorder()
		SettingsGetHandler(store)(rec, newGetRequest())
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body["oidcConfigured"] != false {
			t.Errorf("case %d: oidcConfigured = %v, want false", i, body["oidcConfigured"])
		}
	}
}

func TestSettingsGet_HasPassword(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "false")
	t.Setenv("ENABLE_TRANSLATOR", "false")
	store := fakeStore{st: &fakeStoreState{data: map[string]any{
		"password": "$2a$10$hashedvalue",
	}}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["hasPassword"] != true {
		t.Errorf("hasPassword = %v, want true", body["hasPassword"])
	}
}

func TestSettingsGet_NoPassword(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "false")
	t.Setenv("ENABLE_TRANSLATOR", "false")
	store := fakeStore{st: &fakeStoreState{data: map[string]any{}}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["hasPassword"] != false {
		t.Errorf("hasPassword = %v, want false", body["hasPassword"])
	}
}

func TestSettingsGet_EnvFlagsOn(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "1")
	t.Setenv("ENABLE_TRANSLATOR", "true")
	defer os.Unsetenv("ENABLE_REQUEST_LOGS")
	defer os.Unsetenv("ENABLE_TRANSLATOR")
	store := fakeStore{st: &fakeStoreState{data: map[string]any{}}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["enableRequestLogs"] != true {
		t.Errorf("enableRequestLogs = %v, want true", body["enableRequestLogs"])
	}
	if body["enableTranslator"] != true {
		t.Errorf("enableTranslator = %v, want true", body["enableTranslator"])
	}
}

func TestSettingsGet_MissingRow(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "false")
	t.Setenv("ENABLE_TRANSLATOR", "false")
	store := fakeStore{st: &fakeStoreState{data: nil}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["hasPassword"] != false {
		t.Errorf("hasPassword = %v, want false (no row)", body["hasPassword"])
	}
	if body["oidcConfigured"] != false {
		t.Errorf("oidcConfigured = %v, want false (no row)", body["oidcConfigured"])
	}
}

func TestSettingsGet_Redaction(t *testing.T) {
	t.Setenv("ENABLE_REQUEST_LOGS", "false")
	t.Setenv("ENABLE_TRANSLATOR", "false")
	store := fakeStore{st: &fakeStoreState{data: map[string]any{
		"password":          "should-not-leak",
		"oidcClientSecret":  "should-not-leak",
		"requireLogin":      true,
		"comboStrategy":     "sticky",
	}}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "should-not-leak") {
		t.Errorf("redacted value leaked into response: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "requireLogin") {
		t.Errorf("non-secret field missing from response: %s", bodyStr)
	}
}

func TestSettingsGet_InternalError(t *testing.T) {
	store := fakeStore{st: &fakeStoreState{err: errStoreBroken}}
	rec := httptest.NewRecorder()
	SettingsGetHandler(store)(rec, newGetRequest())
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

// errStoreBroken is a sentinel for the InternalError test.
var errStoreBroken = storeBroken("store exploded")

type storeBroken string

func (s storeBroken) Error() string { return string(s) }
