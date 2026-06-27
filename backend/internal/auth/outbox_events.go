package auth

import "encoding/json"

// GameEndedOutboxPayload serializes a game-ended domain event.
func GameEndedOutboxPayload(payload map[string]interface{}) ([]byte, error) {
	wrapped := map[string]interface{}{
		"event": "game.ended",
		"data":  payload,
	}
	return json.Marshal(wrapped)
}
