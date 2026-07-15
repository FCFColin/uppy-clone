package crypto

import "testing"

func TestLooksLikeBcryptHash(t *testing.T) {
	validBcrypt := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"$2a$ hash", validBcrypt, true},
		{"$2b$ hash", "$2b$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"$2y$ hash", "$2y$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", true},
		{"too short", "$2a$10$abc", false},
		{"too long", validBcrypt + "extra", false},
		{"wrong prefix", "$1a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy", false},
		{"empty string", "", false},
		{"plaintext", "admin123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LooksLikeBcryptHash(tt.input)
			if got != tt.want {
				t.Errorf("LooksLikeBcryptHash(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
