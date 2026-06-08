package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func patchRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPatch, "/api/settings", strings.NewReader(body))
}

func TestSettingsPatch_HappyPath(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{
		"requireLogin": true,
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"requireLogin":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if st.data["requireLogin"] != false {
		t.Errorf("requireLogin = %v, want false", st.data["requireLogin"])
	}
}

func TestSettingsPatch_PasswordChangeFirstTime(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"newPassword":"s3cr3t!"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	hashed, _ := st.data["password"].(string)
	if hashed == "" || hashed == "s3cr3t!" {
		t.Fatalf("password not hashed: %q", hashed)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte("s3cr3t!")); err != nil {
		t.Errorf("bcrypt verify: %v", err)
	}
}

func TestSettingsPatch_PasswordChangeExisting(t *testing.T) {
	existing, _ := bcrypt.GenerateFromPassword([]byte("old"), 10)
	st := &fakeStoreState{data: map[string]any{
		"password": string(existing),
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"newPassword":"newpw","currentPassword":"old"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	hashed, _ := st.data["password"].(string)
	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte("newpw")); err != nil {
		t.Errorf("new bcrypt verify: %v", err)
	}
}

func TestSettingsPatch_MissingCurrentPassword(t *testing.T) {
	existing, _ := bcrypt.GenerateFromPassword([]byte("old"), 10)
	st := &fakeStoreState{data: map[string]any{
		"password": string(existing),
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"newPassword":"newpw"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSettingsPatch_WrongCurrentPassword(t *testing.T) {
	existing, _ := bcrypt.GenerateFromPassword([]byte("old"), 10)
	st := &fakeStoreState{data: map[string]any{
		"password": string(existing),
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"newPassword":"newpw","currentPassword":"not-old"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSettingsPatch_FirstTimeDefaultPassword(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"newPassword":"newpw","currentPassword":"123456"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (default password accepted on first time)", rec.Code)
	}
}

func TestSettingsPatch_OIDCSecretClear(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{
		"oidcClientSecret": "to-be-cleared",
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"oidcClientSecret":""}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := st.data["oidcClientSecret"]; ok {
		t.Errorf("oidcClientSecret should be removed, got %v", st.data["oidcClientSecret"])
	}
}

func TestSettingsPatch_OIDCSecretWhitespace(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{
		"oidcClientSecret": "to-be-cleared",
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"oidcClientSecret":"   "}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if _, ok := st.data["oidcClientSecret"]; ok {
		t.Errorf("oidcClientSecret should be removed on whitespace, got %v", st.data["oidcClientSecret"])
	}
}

func TestSettingsPatch_ProxySideEffect(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{}}
	store := fakeStore{st: st}
	var called int32
	side := SideEffects{
		ApplyOutboundProxyEnv: func(s map[string]any) {
			atomic.AddInt32(&called, 1)
			if s["outboundProxyUrl"] != "http://proxy:8080" {
				t.Errorf("proxy side effect got settings %v", s)
			}
		},
	}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, side)(rec, patchRequest(`{"outboundProxyUrl":"http://proxy:8080"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("ApplyOutboundProxyEnv called %d times, want 1", called)
	}
}

func TestSettingsPatch_ComboSideEffect(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{}}
	store := fakeStore{st: st}
	var called int32
	side := SideEffects{
		ResetComboRotation: func() { atomic.AddInt32(&called, 1) },
	}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, side)(rec, patchRequest(`{"comboStrategy":"sticky"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("ResetComboRotation called %d times, want 1", called)
	}
}

func TestSettingsPatch_PartialUpdate(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{
		"requireLogin":  true,
		"comboStrategy": "round-robin",
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"requireLogin":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if st.data["requireLogin"] != false {
		t.Errorf("requireLogin = %v, want false", st.data["requireLogin"])
	}
	if st.data["comboStrategy"] != "round-robin" {
		t.Errorf("comboStrategy should be untouched, got %v", st.data["comboStrategy"])
	}
}

func TestSettingsPatch_NoPasswordFields(t *testing.T) {
	existing, _ := bcrypt.GenerateFromPassword([]byte("old"), 10)
	st := &fakeStoreState{data: map[string]any{
		"password":  string(existing),
		"tunnelUrl": "https://t.example.com",
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"tunnelUrl":"https://new.example.com"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if st.data["password"] != string(existing) {
		t.Errorf("password changed unexpectedly: %v", st.data["password"])
	}
	if st.data["tunnelUrl"] != "https://new.example.com" {
		t.Errorf("tunnelUrl not updated: %v", st.data["tunnelUrl"])
	}
}

func TestSettingsPatch_MethodNotAllowed(t *testing.T) {
	store := fakeStore{st: &fakeStoreState{}}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestSettingsPatch_InvalidJSON(t *testing.T) {
	store := fakeStore{st: &fakeStoreState{}}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`not json`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestSettingsPatch_ResponseRedaction(t *testing.T) {
	st := &fakeStoreState{data: map[string]any{
		"password":         "hashed-old",
		"oidcClientSecret": "leaky",
	}}
	store := fakeStore{st: st}
	rec := httptest.NewRecorder()
	SettingsPatchHandler(store, SideEffects{})(rec, patchRequest(`{"requireLogin":false}`))
	bodyStr := rec.Body.String()
	if strings.Contains(bodyStr, "hashed-old") {
		t.Errorf("password leaked: %s", bodyStr)
	}
	if strings.Contains(bodyStr, "leaky") {
		t.Errorf("oidcClientSecret leaked: %s", bodyStr)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["requireLogin"] != false {
		t.Errorf("requireLogin = %v, want false", body["requireLogin"])
	}
}
