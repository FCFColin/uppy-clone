// Package game implements multiplayer room state and connection orchestration.
package game

import (
	"fmt"
	"testing"
	"time"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/domain"
)

// cooldownContractCases defines the cross-language cooldown contract.
// Keep in sync with frontend/src/game/cooldown_contract.test.ts.
var cooldownContractCases = []struct {
	PlayerCount int
	ExpectedMs  int64
}{
	{0, 1000}, {-5, 1000}, {1, 1000}, {2, 3032},
	{4, 5064}, {8, 7096}, {100, 14500}, {1000, 15000}, {10000, 15000},
}

func countConnectedPlayers(players map[string]*domain.PlayerState) int {
	n := 0
	for _, p := range players {
		if !p.Disconnected {
			n++
		}
	}
	return n
}

func TestCalculateCooldownContract(t *testing.T) {
	for _, tc := range cooldownContractCases {
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
