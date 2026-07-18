package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/resilience"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
	"golang.org/x/crypto/bcrypt"
)

// TestAdminLoginResponseSchema handler-030: verifies the admin login response
// matches the OpenAPI schema (must contain "message" field as string).
func TestAdminLoginResponseSchema(t *testing.T) {
	resilience.ResetBreakersForTesting()

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	mock := testutil.NewPgxMock(t)

	password := "secret"
	hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	loginCfg, _ := json.Marshal(map[string]string{"admin_password": string(hashed)})

	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}).
			AddRow("global", string(loginCfg), int64(1000)))

	db := store.NewConfigRepository(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewAdminHandler(db, jwtMgr, redisStore)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login",
		strings.NewReader(`{"password":"`+password+`"}`))
	h.Login(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	msg, ok := body["message"].(string)
	if !ok || msg == "" {
		t.Errorf("handler-030: response missing or invalid 'message' field: %+v", body)
	}

	cookie := w.Result().Cookies()
	if len(cookie) == 0 {
		t.Error("handler-030: admin login should set admin_token cookie")
	}
}

// TestAdminLoginBadPasswordResponseSchema handler-030: verifies the admin login
// error response for wrong password matches the OpenAPI error schema.
func TestAdminLoginBadPasswordResponseSchema(t *testing.T) {
	resilience.ResetBreakersForTesting()

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	mock := testutil.NewPgxMock(t)

	hashed, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	loginCfg, _ := json.Marshal(map[string]string{"admin_password": string(hashed)})

	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}).
			AddRow("global", string(loginCfg), int64(1000)))

	db := store.NewConfigRepository(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewAdminHandler(db, jwtMgr, redisStore)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login",
		strings.NewReader(`{"password":"wrong"}`))
	h.Login(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] == nil {
		t.Error("handler-030: error response should include 'status' field per OpenAPI error schema")
	}
}

// TestAdminConfigResponseSchema handler-030: verifies the admin config GET
// response matches the OpenAPI schema.
func TestAdminConfigResponseSchema(t *testing.T) {
	resilience.ResetBreakersForTesting()

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	mock := testutil.NewPgxMock(t)

	cfgJSON, _ := json.Marshal(map[string]interface{}{
		"email_enabled": true,
		"email_from":    "admin@example.com",
	})

	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}).
			AddRow("global", string(cfgJSON), int64(1000)))

	db := store.NewConfigRepository(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewAdminHandler(db, jwtMgr, redisStore)

	token, _, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	r.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
	h.GetConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// OpenAPI spec requires these fields in the config response
	// (AdminConfigResponse schema uses camelCase: emailEnabled, emailFrom)
	expectedFields := []string{"emailEnabled", "emailFrom"}
	for _, field := range expectedFields {
		if _, ok := body[field]; !ok {
			t.Errorf("handler-030: config response missing OpenAPI-required field %q", field)
		}
	}
}

// TestAdminLogoutResponseSchema handler-030: verifies the admin logout response
// matches the OpenAPI schema (must contain "message" field).
func TestAdminLogoutResponseSchema(t *testing.T) {
	resilience.ResetBreakersForTesting()

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewAdminHandler(nil, jwtMgr, redisStore)

	token, jti, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken: %v", err)
	}
	_ = redisStore.AddAdminJTI(context.Background(), jti, 15*time.Minute)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil)
	r.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
	h.Logout(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	msg, ok := body["message"].(string)
	if !ok || msg == "" {
		t.Errorf("handler-030: logout response missing or invalid 'message' field: %+v", body)
	}

	// Verify the admin_token cookie is cleared
	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "admin_token" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("handler-030: logout should clear admin_token cookie")
	}
}
