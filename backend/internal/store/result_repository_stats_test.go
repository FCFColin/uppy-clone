package store

import (
	"context"
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

func TestGetGamesTodayCount(t *testing.T) {
	tests := []struct {
		name      string
		rows      *pgxmock.Rows
		queryErr  error
		wantCount int
		wantErr   bool
	}{
		{
			name:      "success with count",
			rows:      pgxmock.NewRows([]string{"count"}).AddRow(7),
			wantCount: 7,
		},
		{
			name:      "success zero count",
			rows:      pgxmock.NewRows([]string{"count"}).AddRow(0),
			wantCount: 0,
		},
		{
			name:     "query error",
			queryErr: errors.New("db unavailable"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			expect := mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM game_sessions").
				WithArgs(pgxmock.AnyArg())
			if tt.queryErr != nil {
				expect.WillReturnError(tt.queryErr)
			} else {
				expect.WillReturnRows(tt.rows)
			}

			count, err := repo.GetGamesTodayCount(ctx)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("GetGamesTodayCount: %v", err)
			}
			if !tt.wantErr && count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestGetBestScore(t *testing.T) {
	tests := []struct {
		name     string
		rows     *pgxmock.Rows
		queryErr error
		wantBest int
		wantErr  bool
	}{
		{
			name:     "success with max",
			rows:     pgxmock.NewRows([]string{"best"}).AddRow(250),
			wantBest: 250,
		},
		{
			name:     "no records returns zero via coalesce",
			rows:     pgxmock.NewRows([]string{"best"}).AddRow(0),
			wantBest: 0,
		},
		{
			name:     "query error",
			queryErr: errors.New("db unavailable"),
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockRepo(t, NewResultRepository)
			ctx := context.Background()

			expect := mock.ExpectQuery("SELECT COALESCE\\(MAX\\(score_contribution\\), 0\\) FROM game_results")
			if tt.queryErr != nil {
				expect.WillReturnError(tt.queryErr)
			} else {
				expect.WillReturnRows(tt.rows)
			}

			best, err := repo.GetBestScore(ctx)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("GetBestScore: %v", err)
			}
			if !tt.wantErr && best != tt.wantBest {
				t.Errorf("best = %d, want %d", best, tt.wantBest)
			}
		})
	}
}
