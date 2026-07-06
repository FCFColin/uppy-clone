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
	"github.com/redis/go-redis/v9"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/crypto"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

type failLoginLockHook struct{}

func (failLoginLockHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (failLoginLockHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "set" && len(cmd.Args()) > 1 {
			if key, ok := cmd.Args()[1].(string); ok && strings.Contains(key, "admin:login:lock:") {
				return errors.New("set lock failed")
			}
		}
		return next(ctx, cmd)
	}
}

func (failLoginLockHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func TestAdminHandler_Logout(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil)
	h.Logout(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "admin_token" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected admin_token cookie to be cleared")
	}
}

func TestAdminHandler_Logout_WithJTI(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/logout", nil)
	r = r.WithContext(auth.WithJTI(r.Context(), "jti-123"))
	h.Logout(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAdminHandler_Login_PasswordTooLong(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	longPassword := strings.Repeat("x", config.BcryptMaxLen+1)
	w := httptest.NewRecorder()
	body := `{"password":"` + longPassword + `"}`
	h.Login(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAdminHandler_ParseConfigUpdates_InvalidJSON(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader("{bad"))
	_, err := h.parseConfigUpdates(r)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
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

func TestAdminHandler_AuditConfigChange(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", nil)
	before := map[string]interface{}{"email_enabled": false}
	after := map[string]interface{}{"email_enabled": true, "admin_password": "x"}
	h.auditConfigChange(context.Background(), r, before, after)
}

func TestAdminHandler_VerifyAdminTokenClaims(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	token, _, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
	claims, ok := h.VerifyAdminTokenClaims(req)
	if !ok || claims == nil || claims.Role != adminRole {
		t.Fatalf("VerifyAdminTokenClaims = (%v, %v)", claims, ok)
	}
}

func TestAdminHandler_UpdateConfig_InvalidBody(t *testing.T) {
	t.Parallel()

	h := newTestAdminHandler()
	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader("{bad")))
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func newAdminHandlerWithDB(t *testing.T) (*AdminHandler, pgxmock.PgxPoolIface, *store.RedisStore) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	redisStore := testutil.SetupMiniredisStore(t)
	h := NewAdminHandler(db, auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM), redisStore)
	return h, mock, redisStore
}

func expectAdminConfigQuery(mock pgxmock.PgxPoolIface, configJSON string) {
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}).
			AddRow("global", configJSON, int64(1000)))
}

func TestAdminHandler_getStoredAdminPassword(t *testing.T) {
	t.Parallel()
	hashed, _ := hashAdminPassword("secret")
	cfgJSON, _ := json.Marshal(map[string]string{"admin_password": hashed})

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, string(cfgJSON))

	w := httptest.NewRecorder()
	pwd, ok := h.getStoredAdminPassword(context.Background(), w)
	if !ok || pwd != hashed {
		t.Fatalf("getStoredAdminPassword = (%q, %v)", pwd, ok)
	}
}

func TestAdminHandler_getStoredAdminPassword_NotConfigured(t *testing.T) {
	t.Parallel()
	h, mock, _ := newAdminHandlerWithDB(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	_, ok := h.getStoredAdminPassword(context.Background(), w)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, ok = %v", w.Code, ok)
	}
}

func TestAdminHandler_getStoredAdminPassword_InvalidJSON(t *testing.T) {
	t.Parallel()
	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{invalid`)

	w := httptest.NewRecorder()
	_, ok := h.getStoredAdminPassword(context.Background(), w)
	if ok || w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_getStoredAdminPassword_EmptyPassword(t *testing.T) {
	t.Parallel()
	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{"admin_password":""}`)

	w := httptest.NewRecorder()
	_, ok := h.getStoredAdminPassword(context.Background(), w)
	if ok || w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_Login_Success(t *testing.T) {
	password := "admin-pass"
	hashed, _ := hashAdminPassword(password)
	cfgJSON, _ := json.Marshal(map[string]string{"admin_password": hashed})

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, string(cfgJSON))

	body := `{"password":"` + password + `"}`
	w := httptest.NewRecorder()
	h.Login(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_Login_WrongPassword(t *testing.T) {
	hashed, _ := hashAdminPassword("correct")
	cfgJSON, _ := json.Marshal(map[string]string{"admin_password": hashed})

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, string(cfgJSON))

	w := httptest.NewRecorder()
	h.Login(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(`{"password":"wrong"}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_isLoginLocked(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	ctx := context.Background()
	clientIP := "203.0.113.1"
	_ = redisStore.SetLoginLock(ctx, clientIP, "admin", config.AdminTokenTTL)

	w := httptest.NewRecorder()
	if !h.isLoginLocked(ctx, w, clientIP, "admin") {
		t.Fatal("expected locked")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_isLoginLocked_RedisErrorFailClosed(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := httptest.NewRecorder()
	if !h.isLoginLocked(context.Background(), w, "203.0.113.3", "admin") {
		t.Fatal("expected fail-closed on redis error")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
}

func TestAdminHandler_handleFailedLogin_LocksAfterMax(t *testing.T) {
	h, _, _ := newAdminHandlerWithDB(t)
	ctx := context.Background()
	clientIP := "203.0.113.2"
	for i := 0; i < maxFailedLoginAttempts; i++ {
		h.handleFailedLogin(ctx, clientIP, "admin")
	}
	locked, _ := h.redis.IsLoginLocked(ctx, clientIP, "admin")
	if !locked {
		t.Fatal("expected IP locked after max failures")
	}
}

func TestAdminHandler_GetConfig(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto init: %v", err)
	}
	enc, _ := crypto.Encrypt("re_secret")
	cfgJSON, _ := json.Marshal(map[string]interface{}{
		"email_enabled":  true,
		"resend_api_key": enc,
		"email_from":     "a@b.com",
	})

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, string(cfgJSON))

	w := httptest.NewRecorder()
	h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"emailEnabled":true`) {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestAdminHandler_GetConfig_NotFound(t *testing.T) {
	h, mock, _ := newAdminHandlerWithDB(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnRows(pgxmock.NewRows([]string{"id", "config", "updated_at"}))

	w := httptest.NewRecorder()
	h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_UpdateConfig_Success(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	_ = crypto.InitFromEnv()

	h, mock, _ := newAdminHandlerWithDB(t)
	stored := `{"email_enabled":false,"email_from":"old@b.com"}`
	expectAdminConfigQuery(mock, stored)
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs("global", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	body := `{"emailEnabled":true,"emailFrom":"new@b.com"}`
	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_UpdateConfig_ResendApiKey(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	_ = crypto.InitFromEnv()

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{"email_enabled":true}`)
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs("global", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	body := `{"resendApiKey":"re_live_secret_key"}`
	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_UpdateConfig_NotFound(t *testing.T) {
	h, mock, _ := newAdminHandlerWithDB(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(`{"emailEnabled":true}`)))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestAdminHandler_applyConfigUpdates_OldPasswordRequired(t *testing.T) {
	h, _, _ := newAdminHandlerWithDB(t)
	w := httptest.NewRecorder()
	stored := map[string]interface{}{}
	newPwd := "new"
	updates := &configUpdates{AdminPassword: &newPwd}
	if h.applyConfigUpdates(context.Background(), w, httptest.NewRequest(http.MethodPatch, "/", nil), stored, updates) {
		t.Fatal("expected failure")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_applyConfigUpdates_WrongOldPassword(t *testing.T) {
	hashed, _ := hashAdminPassword("correct")
	h, _, _ := newAdminHandlerWithDB(t)
	w := httptest.NewRecorder()
	stored := map[string]interface{}{"admin_password": hashed}
	newPwd := "new"
	oldPwd := "wrong"
	updates := &configUpdates{AdminPassword: &newPwd, OldPassword: &oldPwd}
	if h.applyConfigUpdates(context.Background(), w, httptest.NewRequest(http.MethodPatch, "/", nil), stored, updates) {
		t.Fatal("expected failure")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestAdminHandler_applyConfigUpdates_ChangePassword(t *testing.T) {
	hashed, _ := hashAdminPassword("old-pwd")
	h, _, _ := newAdminHandlerWithDB(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/", nil)
	r = r.WithContext(auth.WithJTI(r.Context(), "jti-1"))
	stored := map[string]interface{}{"admin_password": hashed}
	newPwd := "new-pwd-123"
	oldPwd := "old-pwd"
	updates := &configUpdates{AdminPassword: &newPwd, OldPassword: &oldPwd}
	if !h.applyConfigUpdates(context.Background(), w, r, stored, updates) {
		t.Fatal("expected success")
	}
	if !compareAdminPassword(newPwd, stored["admin_password"].(string)) {
		t.Fatal("password not updated")
	}
}

func TestAuditPasswordChange(t *testing.T) {
	AuditPasswordChange(context.Background(), "127.0.0.1")
}

func TestAdminHandler_GetConfig_InvalidStoredJSON(t *testing.T) {
	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{invalid`)

	w := httptest.NewRecorder()
	h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAdminHandler_VerifyAdminToken_Revoked(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	token, _, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
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

func TestAdminHandler_Login_InvalidBody(t *testing.T) {
	h, _, _ := newAdminHandlerWithDB(t)
	w := httptest.NewRecorder()
	h.Login(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader("{bad")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestAdminHandler_Login_ConfigNotFound(t *testing.T) {
	h, mock, _ := newAdminHandlerWithDB(t)
	mock.ExpectQuery(`SELECT id, config, updated_at FROM admin_config WHERE id = \$1`).
		WithArgs("global").
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	h.Login(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(`{"password":"x"}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAdminHandler_Login_NilRedis(t *testing.T) {
	password := "admin-pass"
	hashed, _ := hashAdminPassword(password)
	cfgJSON, _ := json.Marshal(map[string]string{"admin_password": hashed})

	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock: %v", err)
	}
	t.Cleanup(func() { mock.Close() })
	db := store.NewPostgresStoreWithPool(mock)
	h := NewAdminHandler(db, auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM), nil)
	expectAdminConfigQuery(mock, string(cfgJSON))

	w := httptest.NewRecorder()
	h.Login(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(`{"password":"`+password+`"}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_Logout_RevokeError(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

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
	token, _, err := h.signAdminToken()
	if err != nil {
		t.Fatalf("signAdminToken: %v", err)
	}
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "admin_token", Value: token})
	if _, ok := h.VerifyAdminTokenClaims(req); ok {
		t.Fatal("expected verification failure when redis unavailable")
	}
}

func TestAdminHandler_handleFailedLogin_RedisErrors(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	h.handleFailedLogin(context.Background(), "203.0.113.99", "admin")
}

func TestAdminHandler_GetConfig_DecryptFallback(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	if err := crypto.InitFromEnv(); err != nil {
		t.Fatalf("crypto init: %v", err)
	}
	cfgJSON, _ := json.Marshal(map[string]interface{}{
		"email_enabled":  true,
		"resend_api_key": "not-valid-ciphertext",
		"email_from":     "a@b.com",
	})

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, string(cfgJSON))

	w := httptest.NewRecorder()
	h.GetConfig(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/config", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_UpdateConfig_InvalidStoredJSON(t *testing.T) {
	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{invalid`)
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs("global", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(`{"emailEnabled":true}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAdminHandler_UpdateConfig_SaveError(t *testing.T) {
	t.Setenv("ENCRYPTION_KEY", testsecrets.TestEncryptionKeyHex)
	_ = crypto.InitFromEnv()

	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{"email_enabled":false}`)
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs("global", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnError(context.Canceled)

	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(`{"emailEnabled":true}`)))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAdminHandler_UpdateConfig_SkipMaskedApiKey(t *testing.T) {
	h, mock, _ := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, `{"email_enabled":true,"resend_api_key":"old"}`)
	mock.ExpectExec(`INSERT INTO admin_config`).
		WithArgs("global", pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	body := `{"resendApiKey":"` + maskedKey + `"}`
	w := httptest.NewRecorder()
	h.UpdateConfig(w, httptest.NewRequest(http.MethodPatch, "/api/v1/admin/config", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
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

func TestAdminHandler_applyConfigUpdates_RevokeJWTError(t *testing.T) {
	hashed, _ := hashAdminPassword("old-pwd")
	h, _, redisStore := newAdminHandlerWithDB(t)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/", nil)
	r = r.WithContext(auth.WithJTI(r.Context(), "jti-revoke-err"))
	stored := map[string]interface{}{"admin_password": hashed}
	newPwd := "new-pwd-123"
	oldPwd := "old-pwd"
	updates := &configUpdates{AdminPassword: &newPwd, OldPassword: &oldPwd}
	if !h.applyConfigUpdates(context.Background(), w, r, stored, updates) {
		t.Fatal("expected password change despite revoke error")
	}
}

func TestAdminHandler_saveConfig_MarshalError(t *testing.T) {
	h, _, _ := newAdminHandlerWithDB(t)
	cfg := &domain.AppConfig{ID: "global"}
	err := h.saveConfig(context.Background(), cfg, map[string]interface{}{"bad": make(chan int)})
	if err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestAdminHandler_Login_LockedIP(t *testing.T) {
	hashed, _ := hashAdminPassword("secret")
	cfgJSON, _ := json.Marshal(map[string]string{"admin_password": hashed})
	h, mock, redisStore := newAdminHandlerWithDB(t)
	expectAdminConfigQuery(mock, string(cfgJSON))
	clientIP := "203.0.113.50"
	_ = redisStore.SetLoginLock(context.Background(), clientIP, "admin", time.Minute)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/login", strings.NewReader(`{"password":"secret"}`))
	r.RemoteAddr = clientIP + ":1234"
	h.Login(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
}

func TestAdminHandler_completeAdminLogin_ResetError(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	if err := redisStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	h.completeAdminLogin(w, r, context.Background(), "127.0.0.1", "admin")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

type mockTokenSigner struct {
	err error
}

func (m *mockTokenSigner) SignToken() (string, string, error) {
	return "", "", m.err
}

func TestAdminHandler_completeAdminLogin_SignError(t *testing.T) {
	h := newTestAdminHandler()
	h.tokenSigner = &mockTokenSigner{err: errors.New("sign failed")}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	h.completeAdminLogin(w, r, context.Background(), "127.0.0.1", "admin")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestAdminHandler_handleFailedLogin_SetLockError(t *testing.T) {
	h, _, redisStore := newAdminHandlerWithDB(t)
	ctx := context.Background()
	clientIP := "203.0.113.88"
	for i := 0; i < maxFailedLoginAttempts-1; i++ {
		h.handleFailedLogin(ctx, clientIP, "admin")
	}
	redisStore.Client().AddHook(failLoginLockHook{})
	h.handleFailedLogin(ctx, clientIP, "admin")
}

func TestAdminHandler_completeAdminLogin_SecureCookie(t *testing.T) {
	h := newTestAdminHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "https://example.com/", nil)
	r.TLS = &tls.ConnectionState{}
	h.completeAdminLogin(w, r, context.Background(), "127.0.0.1", "admin")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "admin_token" && !c.Secure {
			t.Fatal("expected secure admin_token cookie for HTTPS request")
		}
	}
}

func TestAdminHandler_handleFailedLogin_NilRedis(t *testing.T) {
	h := newTestAdminHandler()
	h.handleFailedLogin(context.Background(), "127.0.0.1", "admin")
}

func TestAdminHandler_applyConfigUpdates_HashError(t *testing.T) {
	hashed, _ := hashAdminPassword("old-pwd")
	h, _, _ := newAdminHandlerWithDB(t)
	orig := bcryptGenerate
	bcryptGenerate = func(_ []byte, _ int) ([]byte, error) { return nil, errors.New("hash failed") }
	t.Cleanup(func() { bcryptGenerate = orig })

	stored := map[string]interface{}{"admin_password": hashed}
	newPwd := "new-pwd"
	oldPwd := "old-pwd"
	updates := &configUpdates{AdminPassword: &newPwd, OldPassword: &oldPwd}
	w := httptest.NewRecorder()
	if h.applyConfigUpdates(context.Background(), w, httptest.NewRequest(http.MethodPatch, "/", nil), stored, updates) {
		t.Fatal("expected hash error to fail")
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
