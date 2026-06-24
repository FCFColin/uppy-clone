package handler

// 企业为何需要：安全关键组件（中间件/认证/管理）零测试是最高风险——任何改动都可能在生产暴露。

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestCompareAdminPassword_BcryptHash(t *testing.T) {
	password := "admin123"
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt hash error: %v", err)
	}

	t.Run("correct password matches bcrypt hash", func(t *testing.T) {
		if !compareAdminPassword(password, string(hashed)) {
			t.Error("compareAdminPassword should return true for correct password")
		}
	})

	t.Run("wrong password does not match bcrypt hash", func(t *testing.T) {
		if compareAdminPassword("wrong-password", string(hashed)) {
			t.Error("compareAdminPassword should return false for wrong password")
		}
	})
}

func TestCompareAdminPassword_PlaintextRejected(t *testing.T) {
	t.Run("plaintext password is rejected (no fallback)", func(t *testing.T) {
		if compareAdminPassword("admin123", "admin123") {
			t.Error("compareAdminPassword should return false for plaintext stored password")
		}
	})

	t.Run("wrong plaintext password is rejected", func(t *testing.T) {
		if compareAdminPassword("wrong", "admin123") {
			t.Error("compareAdminPassword should return false for non-bcrypt stored password")
		}
	})
}

func TestHashAdminPassword(t *testing.T) {
	t.Run("produces bcrypt hash", func(t *testing.T) {
		hashed, err := hashAdminPassword("admin123")
		if err != nil {
			t.Fatalf("hashAdminPassword error: %v", err)
		}
		if !isBcryptHash(hashed) {
			t.Errorf("hashAdminPassword should produce bcrypt hash, got %q", hashed)
		}
	})

	t.Run("different passwords produce different hashes", func(t *testing.T) {
		hash1, _ := hashAdminPassword("password1")
		hash2, _ := hashAdminPassword("password2")
		if hash1 == hash2 {
			t.Error("different passwords should produce different hashes")
		}
	})

	t.Run("same password produces different hashes (salt)", func(t *testing.T) {
		hash1, _ := hashAdminPassword("same-password")
		hash2, _ := hashAdminPassword("same-password")
		if hash1 == hash2 {
			t.Error("same password should produce different hashes due to salt")
		}
	})

	t.Run("hashed password can be verified", func(t *testing.T) {
		password := "test-password-123"
		hashed, err := hashAdminPassword(password)
		if err != nil {
			t.Fatalf("hashAdminPassword error: %v", err)
		}
		if !compareAdminPassword(password, hashed) {
			t.Error("hashed password should be verifiable with compareAdminPassword")
		}
	})
}

func TestIsBcryptHash(t *testing.T) {
	// bcrypt hashes are always 60 characters: $2a$10$ + 53 chars
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
		{"wrong prefix", "$1a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhW", false},
		{"empty string", "", false},
		{"plaintext", "admin123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBcryptHash(tt.input)
			if got != tt.want {
				t.Errorf("isBcryptHash(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestChangePassword_FullFlow(t *testing.T) {
	t.Run("change password with correct old password", func(t *testing.T) {
		oldPassword := "old-password"
		oldHash, _ := hashAdminPassword(oldPassword)

		// Verify old password works
		if !compareAdminPassword(oldPassword, oldHash) {
			t.Fatal("old password should match old hash")
		}

		// Change to new password
		newPassword := "new-password"
		newHash, err := hashAdminPassword(newPassword)
		if err != nil {
			t.Fatalf("hashAdminPassword error: %v", err)
		}

		// Verify new password works
		if !compareAdminPassword(newPassword, newHash) {
			t.Error("new password should match new hash")
		}

		// Verify old password no longer works with new hash
		if compareAdminPassword(oldPassword, newHash) {
			t.Error("old password should not match new hash")
		}
	})

	t.Run("change password with wrong old password", func(t *testing.T) {
		correctPassword := "correct-password"
		storedHash, _ := hashAdminPassword(correctPassword)

		// Try to verify with wrong password
		if compareAdminPassword("wrong-password", storedHash) {
			t.Error("wrong password should not match stored hash")
		}
	})
}
