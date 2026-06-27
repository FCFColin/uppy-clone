package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
)

type mockUserDataStore struct {
	user    *domain.User
	userErr error
	results []domain.GameResult
	resultsErr error
	anonymizeErr error
}

func (m *mockUserDataStore) GetUserByID(_ context.Context, _ string) (*domain.User, error) {
	return m.user, m.userErr
}

func (m *mockUserDataStore) AnonymizeUser(_ context.Context, _ string) error {
	return m.anonymizeErr
}

func (m *mockUserDataStore) GetGameResultsByUserID(_ context.Context, _ string) ([]domain.GameResult, error) {
	return m.results, m.resultsErr
}

func TestExportUserData_Success(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{
		user: &domain.User{ID: "u1", Email: "test@example.com", Nickname: "Test"},
		results: []domain.GameResult{{ID: "r1", UserID: "u1"}},
	}
	data, err := ExportUserData(context.Background(), store, "u1")
	if err != nil {
		t.Fatalf("ExportUserData: %v", err)
	}
	if data["user"] == nil {
		t.Error("export should contain user")
	}
	if data["game_results"] == nil {
		t.Error("export should contain game_results")
	}
}

func TestExportUserData_UserNotFound(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{user: nil, userErr: nil}
	_, err := ExportUserData(context.Background(), store, "nonexistent")
	if err == nil {
		t.Fatal("ExportUserData should return error for nil user")
	}
}

func TestExportUserData_StoreError(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{userErr: errors.New("db down")}
	_, err := ExportUserData(context.Background(), store, "u1")
	if err == nil {
		t.Fatal("ExportUserData should return error when store fails")
	}
}

func TestExportUserData_NoGameResults(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{
		user:    &domain.User{ID: "u1", Email: "a@b.com", Nickname: "A"},
		results: nil,
	}
	data, err := ExportUserData(context.Background(), store, "u1")
	if err != nil {
		t.Fatalf("ExportUserData: %v", err)
	}
	results, ok := data["game_results"]
	if !ok {
		t.Error("export should contain game_results key")
	}
	if results == nil {
		t.Error("game_results should not be nil even when empty")
	}
}

func TestExportUserData_GameResultsError(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{
		user:      &domain.User{ID: "u1", Email: "a@b.com", Nickname: "A"},
		resultsErr: errors.New("query failed"),
	}
	data, err := ExportUserData(context.Background(), store, "u1")
	if err != nil {
		t.Fatalf("ExportUserData should not fail on game results error: %v", err)
	}
	if data["game_results"] == nil {
		t.Error("export should have empty game_results on error")
	}
}

func TestDeleteUserData_NilDataStore(t *testing.T) {
	t.Parallel()
	err := DeleteUserData(context.Background(), nil, nil, nil, nil, "u1", nil)
	if err != nil {
		t.Errorf("DeleteUserData with nil dataStore should succeed: %v", err)
	}
}

func TestDeleteUserData_AnonymizeError(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{anonymizeErr: errors.New("anonymize failed")}
	err := DeleteUserData(context.Background(), nil, nil, nil, store, "u1", nil)
	if err == nil {
		t.Fatal("DeleteUserData should return error when anonymize fails")
	}
}

func TestDeleteUserData_AnonymizeSuccess(t *testing.T) {
	t.Parallel()
	store := &mockUserDataStore{}
	err := DeleteUserData(context.Background(), nil, nil, nil, store, "u1", nil)
	if err != nil {
		t.Errorf("DeleteUserData should succeed: %v", err)
	}
}
