package handler

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestAdminHandler_Logout(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil)
	h.Logout(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "admin_token" && c.MaxAge < 0 {
			return
		}
	}
	t.Error("expected admin_token cookie to be cleared")
}

func TestMaskSensitiveFields(t *testing.T) {
	t.Parallel()

	in := map[string]interface{}{
		"admin_password": "secret",
		"resend_api_key": "re_key",
		"email_from":     "a@b.com",
	}
	out := maskSensitiveFields(in)
	if out["admin_password"] != maskedKey || out["resend_api_key"] != maskedKey {
		t.Errorf("sensitive fields not masked: %+v", out)
	}
	if out["email_from"] != "a@b.com" {
		t.Errorf("non-sensitive field altered: %v", out["email_from"])
	}
}

func TestAdminHandler_getStoredAdminPassword(t *testing.T) {
	t.Parallel()

	hashed, _ := hashAdminPassword("secret")
	hashedCfg, _ := json.Marshal(map[string]string{"admin_password": hashed})

	tests := []struct {
		name       string
		cfgJSON    string
		dbError    bool
		wantOK     bool
		wantPwd    string
		wantStatus int
	}{
		{"success", string(hashedCfg), false, true, hashed, 0},
		{"db error not configured", "", true, false, "", http.StatusForbidden},
		{"invalid json", `{invalid`, false, false, "", http.StatusInternalServerError},
		{"empty password", `{"admin_password":""}`, false, false, "", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, mock, _ := newAdminHandlerWithDB(t)
			if tt.dbError {
				expectAdminConfigQueryError(mock, context.Canceled)
			} else {
				expectAdminConfigQuery(mock, tt.cfgJSON)
			}

			w := httptest.NewRecorder()
			pwd, ok := h.getStoredAdminPassword(context.Background(), w)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && pwd != tt.wantPwd { // pragma: allowlist secret
				t.Fatalf("pwd = %q, want %q", pwd, tt.wantPwd)
			}
			if !tt.wantOK && w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// Login uses switch-based setup to mirror getStoredAdminPassword's style.
func TestAdminHandler_Login(t *testing.T) {
	t.Parallel()

	correctHashed, _ := hashAdminPassword("correct")
	cfgCorrect, _ := json.Marshal(map[string]string{"admin_password": correctHashed})

	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"password too long", `{"password":"` + strings.Repeat("x", config.BcryptMaxLen+1) + `"}`, http.StatusBadRequest},
		{"invalid body", "{bad", http.StatusBadRequest},
		{"success", `{"password":"correct"}`, http.StatusOK},
		{"wrong password", `{"password":"wrong"}`, http.StatusUnauthorized},
		{"config not found", `{"password":"x"}`, http.StatusForbidden},
		// handler-001: nil Redis → fail-closed → 503 (login denied for security).
		{"nil redis", `{"password":"correct"}`, http.StatusServiceUnavailable},
		{"locked ip", `{"password":"correct"}`, http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h *AdminHandler
			var r *http.Request
			var mock pgxmock.PgxPoolIface
			var redisStore *store.RedisStore
			switch tt.name {
			case "password too long", "invalid body":
				h = newTestAdminHandler()
			case "nil redis":
				mock = testutil.NewPgxMock(t)
				db := store.NewConfigRepository(mock)
				h = NewAdminHandler(db, newTestJWTManager(), nil)
				expectAdminConfigQuery(mock, string(cfgCorrect))
			case "locked ip":
				h, mock, redisStore = newAdminHandlerWithDB(t)
				expectAdminConfigQuery(mock, string(cfgCorrect))
				_ = redisStore.SetLoginLock(context.Background(), "203.0.113.50", "admin", time.Minute)
				r = httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(tt.body))
				r.RemoteAddr = "203.0.113.50:1234"
			default:
				h, mock, _ = newAdminHandlerWithDB(t)
				if tt.name == "config not found" {
					expectAdminConfigQueryError(mock, context.Canceled)
				} else {
					expectAdminConfigQuery(mock, string(cfgCorrect))
				}
			}
			if r == nil {
				r = httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(tt.body))
			}
			w := httptest.NewRecorder()
			h.Login(w, r)
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d body=%s", w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

func TestAdminHandler_isLoginLocked(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, redisStore *store.RedisStore)
		wantCode int
	}{
		{"locked", func(_ *testing.T, redisStore *store.RedisStore) {
			_ = redisStore.SetLoginLock(context.Background(), "203.0.113.1", "admin", config.AdminTokenTTL)
		}, http.StatusTooManyRequests},
		{"redis error fail closed", func(t *testing.T, redisStore *store.RedisStore) {
			mustCloseRedis(t, redisStore)
		}, http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, redisStore := newAdminHandlerWithDB(t)
			tt.setup(t, redisStore)
			w := httptest.NewRecorder()
			if !h.isLoginLocked(context.Background(), w, "203.0.113.1", "admin") {
				t.Fatal("expected locked")
			}
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

func TestAdminHandler_handleFailedLogin_LocksAfterMax(t *testing.T) {
	h, _, _ := newAdminHandlerWithDB(t)
	ctx := context.Background()
	clientIP := "203.0.113.2"
	for i := 0; i < maxFailedLoginAttempts; i++ {
		h.handleFailedLogin(ctx, clientIP, "admin")
	}
	if locked, _ := h.redis.IsLoginLocked(ctx, clientIP, "admin"); !locked {
		t.Fatal("expected IP locked after max failures")
	}
}

func TestAdminHandler_GetConfig(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto init: %v", err)
	}
	enc, _ := crypto.Encrypt("re_secret")

	mkCfg := func(apiKey interface{}) string {
		b, _ := json.Marshal(map[string]interface{}{
			"email_enabled":  true,
			"resend_api_key": apiKey,
			"email_from":     "a@b.com",
		})
		return string(b)
	}

	tests := []struct {
		name      string
		cfgJSON   string
		emptyRows bool
		wantCode  int
		wantBody  string
	}{
		{"success", mkCfg(enc), false, http.StatusOK, `"emailEnabled":true`},
		{"not found", "", true, http.StatusNotFound, ""},
		{"invalid stored json", `{invalid`, false, http.StatusInternalServerError, ""},
		{"decrypt fallback", mkCfg("not-valid-ciphertext"), false, http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, mock, _ := newAdminHandlerWithDB(t)
			if tt.emptyRows {
				expectAdminConfigQuery(mock, "", true)
			} else {
				expectAdminConfigQuery(mock, tt.cfgJSON)
			}
			w := httptest.NewRecorder()
			h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil))
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d body=%s", w.Code, tt.wantCode, w.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Fatalf("body = %s, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestAdminHandler_UpdateConfig(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	_ = crypto.InitFromEnv()

	tests := []struct {
		name      string
		body      string
		storedCfg string
		emptyRows bool
		saveErr   error
		wantCode  int
	}{
		{"invalid body", "{bad", "", false, nil, http.StatusBadRequest},
		{"success", `{"emailEnabled":true,"emailFrom":"new@b.com"}`, `{"email_enabled":false,"email_from":"old@b.com"}`, false, nil, http.StatusOK},
		{"resend api key", `{"resendApiKey":"re_live_secret_key"}`, `{"email_enabled":true}`, false, nil, http.StatusOK},
		{"not found", `{"emailEnabled":true}`, "", true, nil, http.StatusNotFound},
		{"invalid stored json", `{"emailEnabled":true}`, `{invalid`, false, nil, http.StatusOK},
		{"save error", `{"emailEnabled":true}`, `{"email_enabled":false}`, false, context.Canceled, http.StatusInternalServerError},
		{"skip masked api key", `{"resendApiKey":"` + maskedKey + `"}`, `{"email_enabled":true,"resend_api_key":"old"}`, false, nil, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, mock, _ := newAdminHandlerWithDB(t)
			if tt.emptyRows {
				expectAdminConfigQuery(mock, "", true)
			} else {
				expectAdminConfigQuery(mock, tt.storedCfg)
			}
			// Save is expected only for success (200) or save-error (500) paths;
			// invalid-body (400) and not-found (404) short-circuit before save.
			if tt.wantCode == http.StatusOK {
				expectAdminConfigSave(mock)
			} else if tt.saveErr != nil {
				mock.ExpectExec(`INSERT INTO admin_config`).
					WithArgs("global", pgxmock.AnyArg(), pgxmock.AnyArg()).
					WillReturnError(tt.saveErr)
			}
			w := httptest.NewRecorder()
			h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(tt.body)))
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d body=%s", w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

func TestAdminHandler_UpdateConfig_EncryptError(t *testing.T) {
	crypto.ResetKeyForTest()
	t.Cleanup(func() {
		t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
		_ = crypto.InitFromEnv()
	})

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{"email_enabled":true}`)
	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(`{"resendApiKey":"re_live_secret_key"}`)))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAdminHandler_applyConfigUpdates(t *testing.T) {
	tests := []struct {
		name       string
		storedPwd  string // empty means no stored password
		newPwd     string
		oldPwd     string
		jti        string // empty = no JTI in context
		setup      func(t *testing.T, h *AdminHandler, redisStore *store.RedisStore)
		wantStatus int // 0 means success (applyConfigUpdates returns true)
	}{
		{"old password required", "", "new", "", "", nil, http.StatusBadRequest},
		{"wrong old password", hashMustPwd("correct"), "new", "wrong", "", nil, http.StatusUnauthorized},
		{"change password", hashMustPwd("old-pwd"), "new-pwd-123", "old-pwd", "jti-1", nil, 0},
		{"revoke jwt error tolerated", hashMustPwd("old-pwd"), "new-pwd-123", "old-pwd", "jti-revoke-err", func(t *testing.T, _ *AdminHandler, redisStore *store.RedisStore) {
			mustCloseRedis(t, redisStore)
		}, 0},
		{"hash error", hashMustPwd("old-pwd"), "new-pwd", "old-pwd", "", func(t *testing.T, _ *AdminHandler, _ *store.RedisStore) {
			orig := bcryptGenerate
			bcryptGenerate = func(_ []byte, _ int) ([]byte, error) { return nil, errors.New("hash failed") }
			t.Cleanup(func() { bcryptGenerate = orig })
		}, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, redisStore := newAdminHandlerWithDB(t)
			if tt.setup != nil {
				tt.setup(t, h, redisStore)
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPatch, "/", nil)
			if tt.jti != "" {
				r = r.WithContext(auth.WithJTI(r.Context(), tt.jti))
			}
			stored := map[string]interface{}{}
			if tt.storedPwd != "" {
				stored["admin_password"] = tt.storedPwd // pragma: allowlist secret
			}
			updates := &configUpdates{AdminPassword: &tt.newPwd}
			if tt.oldPwd != "" {
				updates.OldPassword = &tt.oldPwd
			}
			ok := h.applyConfigUpdates(context.Background(), w, r, stored, updates)
			if tt.wantStatus == 0 {
				if !ok || !compareAdminPassword(tt.newPwd, stored["admin_password"].(string)) {
					t.Fatal("expected success with password updated")
				}
			} else if ok || w.Code != tt.wantStatus {
				t.Fatalf("ok=%v status = %d, want %d", ok, w.Code, tt.wantStatus)
			}
		})
	}
}

func TestAdminHandler_VerifyAdminToken_Revoked(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	req := newAdminTokenRequest(t, h)
	claims, ok := h.VerifyAdminTokenClaims(req)
	if !ok {
		t.Fatal("expected valid token before revocation")
	}
	if err := redisStore.RevokeJWT(context.Background(), claims.ID, time.Minute); err != nil {
		t.Fatalf("RevokeJWT: %v", err)
	}
	if h.VerifyAdminToken(req) {
		t.Fatal("revoked token should fail verification")
	}
}

func TestAdminHandler_Logout_RevokeError(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	mustCloseRedis(t, redisStore)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil)
	r = r.WithContext(auth.WithJTI(r.Context(), "jti-logout"))
	h.Logout(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestAdminHandler_VerifyAdminTokenClaims_RedisError(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	req := newAdminTokenRequest(t, h)
	mustCloseRedis(t, redisStore)
	if _, ok := h.VerifyAdminTokenClaims(req); ok {
		t.Fatal("expected verification failure when redis unavailable")
	}
}

func TestAdminHandler_completeAdminLogin(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (*AdminHandler, *http.Request)
		wantCode    int
		checkSecure bool
	}{
		{"reset error tolerated", func(t *testing.T) (*AdminHandler, *http.Request) {
			h, _, redisStore := newAdminHandlerWithDB(t)
			mustCloseRedis(t, redisStore)
			return h, httptest.NewRequest(http.MethodPost, "/", nil)
		}, http.StatusOK, false},
		{"sign error", func(_ *testing.T) (*AdminHandler, *http.Request) {
			h := newTestAdminHandler()
			h.tokenSigner = mockTokenSignerFn(errors.New("sign failed"))
			return h, httptest.NewRequest(http.MethodPost, "/", nil)
		}, http.StatusInternalServerError, false},
		{"secure cookie on https", func(_ *testing.T) (*AdminHandler, *http.Request) {
			h := newTestAdminHandler()
			r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
			r.TLS = &tls.ConnectionState{}
			return h, r
		}, http.StatusOK, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, r := tt.setup(t)
			w := httptest.NewRecorder()
			h.completeAdminLogin(w, r, context.Background(), "127.0.0.1", "admin")
			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if tt.checkSecure {
				for _, c := range w.Result().Cookies() {
					if c.Name == "admin_token" && !c.Secure {
						t.Fatal("expected secure admin_token cookie for HTTPS request")
					}
				}
			}
		})
	}
}
