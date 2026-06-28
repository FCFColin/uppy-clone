package game

import "github.com/uppy-clone/backend/internal/domain"

func countConnectedPlayers(players map[string]*domain.PlayerState) int {
	n := 0
	for _, p := range players {
		if !p.Disconnected {
			n++
		}
	}
	return n
}

func hasAnyConnectedPlayer(players map[string]*domain.PlayerState) bool {
	for _, p := range players {
		if !p.Disconnected {
			return true
		}
	}
	return false
}

func countRestartYesVotes(players map[string]*domain.PlayerState, votes map[string]bool) (yes, connected int) {
	for _, p := range players {
		if !p.Disconnected {
			connected++
			if v, ok := votes[p.ID]; ok && v {
				yes++
			}
		}
	}
	return yes, connected
}
