package game

import "github.com/uppy-clone/backend/internal/domain"

func anyPlayerConnected(players map[string]*domain.PlayerState) bool {
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
