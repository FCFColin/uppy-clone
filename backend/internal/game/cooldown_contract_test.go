package game

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

type cooldownContract struct {
	Cases []struct {
		PlayerCount int   `json:"playerCount"`
		ExpectedMs  int64 `json:"expectedMs"`
	} `json:"cases"`
}

func TestCalculateCooldownContract(t *testing.T) {
	root := findRepoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "shared", "data", "cooldown_contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract cooldownContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	for _, tc := range contract.Cases {
		t.Run(fmt.Sprintf("count_%d", tc.PlayerCount), func(t *testing.T) {
			t.Parallel()
			got := CalculateCooldown(tc.PlayerCount)
			if got != tc.ExpectedMs {
				t.Fatalf("CalculateCooldown(%d) = %d, want %d", tc.PlayerCount, got, tc.ExpectedMs)
			}
		})
	}
}

func TestUpdatePlayerStats_CooldownUsesRosterSize(t *testing.T) {
	r := NewRoom("CDR1", nil, nil, config.DefaultTimeoutConfig(), 0)
	now := time.Now().UnixMilli()
	disconnectedAt := now - 1000
	r.state.Players = map[string]*domain.PlayerState{
		"p1": {ID: "p1", PlayerIndex: 0},
		"p2": {ID: "p2", PlayerIndex: 1, Disconnected: true, DisconnectedAt: &disconnectedAt},
	}
	player := r.state.Players["p1"]

	got := r.updatePlayerStats(player, now)
	want := CalculateCooldown(len(r.state.Players))
	if got != want {
		t.Fatalf("cooldown = %d, want %d (roster size %d)", got, want, len(r.state.Players))
	}
	connectedOnly := CalculateCooldown(countConnectedPlayers(r.state.Players))
	if got == connectedOnly && len(r.state.Players) != countConnectedPlayers(r.state.Players) {
		t.Fatalf("cooldown matched connected-only count unexpectedly")
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "shared", "data", "cooldown_contract.json")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
