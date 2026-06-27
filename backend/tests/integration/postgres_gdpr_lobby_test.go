//go:build integration

package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/testutil"
)

func TestPostgresStore_AnonymizeUser(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	email := fmt.Sprintf("gdpr-%d@example.com", time.Now().UnixNano())
	user := &domain.User{
		ID:        uuid.NewString(),
		Email:     email,
		Nickname:  "GDPRUser",
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := db.AnonymizeUser(ctx, user.ID); err != nil {
		t.Fatalf("AnonymizeUser: %v", err)
	}
	got, err := db.GetUserByID(ctx, user.ID)
	if err != nil || got == nil {
		t.Fatalf("GetUserByID after anonymize: %v", err)
	}
	if got.Email != "" && got.Nickname == "GDPRUser" {
		t.Fatalf("expected anonymized user, got %+v", got)
	}
}

func TestPostgresStore_LoadAllActiveLobbiesCursor(t *testing.T) {
	db := testutil.SetupPostgresStore(t)
	ctx := context.Background()

	for i, code := range []string{"AAAAA", "BBBBB", "CCCCC"} {
		ls := &domain.LobbyState{
			ID:        uuid.NewString(),
			Code:      code,
			State:     `{}`,
			UpdatedAt: int64(1000 - i),
			CreatedAt: time.Now().UnixMilli(),
		}
		if err := db.SaveLobbyState(ctx, ls); err != nil {
			t.Fatalf("SaveLobbyState: %v", err)
		}
	}

	page1, err := db.LoadAllActiveLobbies(ctx, 2, "")
	if err != nil || len(page1.Lobbies) != 2 || !page1.HasMore {
		t.Fatalf("page1: lobbies=%d hasMore=%v err=%v", len(page1.Lobbies), page1.HasMore, err)
	}
	page2, err := db.LoadAllActiveLobbies(ctx, 2, page1.NextCursor)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Lobbies) < 1 {
		t.Fatalf("expected remaining lobbies, got %d", len(page2.Lobbies))
	}
}
