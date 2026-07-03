//go:build integration

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestQuickPlayResponseMatchesOpenAPISchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := testutil.SetupPostgresStore(t)
	_, rdb := testutil.SetupRedisStore(t)

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTSecret)
	refreshMgr := auth.NewRefreshTokenManager(rdb.Client())
	timeouts := config.DefaultTimeoutConfig()

	authSvc := newMockAuthSvc(jwtMgr, refreshMgr, rdb, db, "", "", timeouts)
	h := NewAuthHandler(db, rdb, authSvc, &Config{})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/quickplay", strings.NewReader(`{"nickname":"SchemaTest"}`))
	r.Header.Set("Content-Type", "application/json")
	h.QuickPlay(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	expectedFields := []string{"userId"}
	for _, field := range expectedFields {
		if _, ok := body[field]; !ok {
			t.Errorf("response missing OpenAPI-required field %q", field)
		}
	}

	userID, ok := body["userId"].(string)
	if !ok || userID == "" {
		t.Error("userId must be a non-empty string per OpenAPI spec")
	}
	if nickname != "SchemaTest" {
		t.Errorf("nickname = %q, want %q", nickname, "SchemaTest")
	}

	if _, ok := body["refreshToken"]; ok {
		t.Error("refreshToken should not appear in response body (json:\"-\")")
	}
}