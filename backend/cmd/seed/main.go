// Package seed inserts test data for development environments.
//
// 企业为何需要：开发环境需要可重复的测试数据，手动创建易出错且耗时。
// 安全：仅在 sslmode=disable（开发标识）时执行，防止误操作生产数据库。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/idgen"
	"github.com/uppy-clone/backend/internal/store"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL required")
	}

	// Safety check: only run in dev (sslmode=disable).
	// 企业为何需要：防止 seed 脚本误操作生产数据库，造成测试数据污染。
	if !strings.Contains(dbURL, "sslmode=disable") {
		log.Fatal("SEED ABORTED: DATABASE_URL must contain sslmode=disable (dev only)")
	}

	timeouts := config.DefaultTimeoutConfig()
	db, err := store.NewPostgresStore(dbURL, timeouts)
	if err != nil {
		log.Fatalf("connect DB: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().Unix()

	users := seedUsers(ctx, db, now)
	sessionIDs := seedSessions(ctx, db, now)
	seedResults(ctx, db, now, users, sessionIDs)

	log.Println("Seed completed: 3 users, 5 game sessions, 10 game results")
}

// seedUsers inserts 3 test users and returns them for use by other seed helpers.
func seedUsers(ctx context.Context, db *store.PostgresStore, now int64) []*domain.User {
	users := []*domain.User{
		{ID: idgen.UUID(), Email: "alice@test.com", Nickname: "Alice", Palette: 0, CreatedAt: now},
		{ID: idgen.UUID(), Email: "bob@test.com", Nickname: "Bob", Palette: 1, CreatedAt: now},
		{ID: idgen.UUID(), Email: "charlie@test.com", Nickname: "Charlie", Palette: 2, CreatedAt: now},
	}
	for _, u := range users {
		if err := db.CreateUser(ctx, u); err != nil {
			log.Printf("create user %s: %v (may already exist)", u.Nickname, err)
		}
	}
	return users
}

// seedSessions inserts 5 completed game sessions and returns their IDs.
func seedSessions(ctx context.Context, db *store.PostgresStore, now int64) []string {
	sessionIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		code := fmt.Sprintf("SEED%d", i)
		endedAt := now - int64(i*3600)
		startedAt := endedAt - 600
		session := &domain.GameSession{
			ID:         idgen.UUID(),
			LobbyCode:  code,
			Status:     "completed",
			StartedAt:  &startedAt,
			EndedAt:    &endedAt,
			FinalScore: 1000 - i*100,
		}
		sessionIDs[i] = session.ID
		if err := db.CreateGameSession(ctx, session); err != nil {
			log.Printf("create game session %s: %v (may already exist)", code, err)
		}
	}
	return sessionIDs
}

// seedResults inserts 10 game results (2 per session, cycling through users).
func seedResults(ctx context.Context, db *store.PostgresStore, now int64, users []*domain.User, sessionIDs []string) {
	pool := db.Pool()
	for i := 0; i < 10; i++ {
		sessionIdx := i / 2
		userIdx := i % len(users)
		// Bounds checks (defensive): sessionIdx and userIdx are derived from
		// fixed ranges, but guard against slice out-of-range to satisfy gosec.
		if sessionIdx >= len(sessionIDs) {
			sessionIdx = 0
		}
		if userIdx >= len(users) {
			userIdx = 0
		}
		result := &domain.GameResult{
			ID:                idgen.UUID(),
			SessionID:         sessionIDs[sessionIdx],
			UserID:            users[userIdx].ID,
			ScoreContribution: 500 - i*30,
			TapsCount:         20 + i*3,
			CreatedAt:         now - int64(i*60),
		}
		_, err := pool.Exec(ctx,
			`INSERT INTO game_results (id, session_id, user_id, score_contribution, taps_count, created_at) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (id) DO NOTHING`,
			result.ID, result.SessionID, result.UserID, result.ScoreContribution, result.TapsCount, result.CreatedAt)
		if err != nil {
			log.Printf("create game result %d: %v (may already exist)", i, err)
		}
	}
}
