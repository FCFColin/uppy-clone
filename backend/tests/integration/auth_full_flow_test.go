//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/store"
	"github.com/uppy-clone/backend/internal/testsecrets"
	"github.com/uppy-clone/backend/internal/testutil"
)

// RO-037: Pure JWT tests and miniredis-based refresh-token tests were moved to
// internal/auth/auth_token_test.go (unit tests, no external deps).
// This file retains only tests that require a real Postgres testcontainer.

func TestAuth_QuickPlayWithRealDB(t *testing.T) {
	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	redisStore := testutil.SetupMiniredisStore(t)
	userRepo := store.NewUserRepository(db.Pool())

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"IntegrationPlayer"}`))

	cookie, resp, err := auth.QuickPlay(userRepo, jwtMgr, refreshMgr, nil, "IntegrationPlayer", req)
	if err != nil {
		t.Fatalf("QuickPlay: %v", err)
	}
	if cookie == nil {
		t.Fatal("expected non-nil cookie")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.UserID == "" {
		t.Fatal("expected non-empty UserID")
	}
	if resp.Nickname == "" {
		t.Fatal("expected non-empty Nickname")
	}
}

func TestAuth_QuickPlayExistingSession(t *testing.T) {
	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	redisStore := testutil.SetupMiniredisStore(t)
	userRepo := store.NewUserRepository(db.Pool())

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	firstReq := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"First"}`))
	_, firstResp, err := auth.QuickPlay(userRepo, jwtMgr, refreshMgr, nil, "First", firstReq)
	if err != nil {
		t.Fatalf("first QuickPlay: %v", err)
	}

	token, err := jwtMgr.SignToken(firstResp.UserID, firstResp.Nickname)
	if err != nil {
		t.Fatalf("SignToken: %v", err)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"Second"}`))
	secondReq.AddCookie(&http.Cookie{Name: "quickplay", Value: token})

	_, secondResp, err := auth.QuickPlay(userRepo, jwtMgr, refreshMgr, nil, "Second", secondReq)
	if err != nil {
		t.Fatalf("second QuickPlay: %v", err)
	}
	if secondResp.UserID != firstResp.UserID {
		t.Fatalf("UserID = %q, want %q (same user on existing session)", secondResp.UserID, firstResp.UserID)
	}
}

func TestAuth_ConcurrentQuickplay(t *testing.T) {
	db := testutil.SetupPostgres(t, testutil.WithStore(), testutil.WithMigrations()).Store
	redisStore := testutil.SetupMiniredisStore(t)
	userRepo := store.NewUserRepository(db.Pool())

	jwtMgr := auth.NewJWTManager(testsecrets.TestJWTPrivateKeyPEM)
	refreshMgr := auth.NewRefreshTokenManager(redisStore.Client())

	const goroutines = 5
	var wg sync.WaitGroup
	var mu sync.Mutex
	userIDs := make(map[string]bool)
	var errs []error

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			nickname := fmt.Sprintf("ConcurrentPlayer%d", idx)
			req := httptest.NewRequest(http.MethodPost, "https://example.com/quickplay", strings.NewReader(`{"nickname":"`+nickname+`"}`))

			cookie, resp, err := auth.QuickPlay(userRepo, jwtMgr, refreshMgr, nil, nickname, req)
			mu.Lock()
			if err != nil {
				errs = append(errs, fmt.Errorf("goroutine %d: %w", idx, err))
				mu.Unlock()
				return
			}
			if cookie == nil {
				errs = append(errs, fmt.Errorf("goroutine %d: nil cookie", idx))
				mu.Unlock()
				return
			}
			if resp == nil {
				errs = append(errs, fmt.Errorf("goroutine %d: nil response", idx))
				mu.Unlock()
				return
			}
			if resp.UserID == "" {
				errs = append(errs, fmt.Errorf("goroutine %d: empty UserID", idx))
				mu.Unlock()
				return
			}
			if userIDs[resp.UserID] {
				errs = append(errs, fmt.Errorf("goroutine %d: duplicate UserID %q", idx, resp.UserID))
			}
			userIDs[resp.UserID] = true
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if len(userIDs) != goroutines {
		t.Fatalf("expected %d unique user IDs, got %d", goroutines, len(userIDs))
	}
}
