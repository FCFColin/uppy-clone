package handler

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

// bcryptGenerate is replaceable in unit tests to simulate hashing failures.
var bcryptGenerate = bcrypt.GenerateFromPassword

// compareAdminPassword compares a plaintext password against a stored hash.
// Only bcrypt hashes are supported — legacy plaintext fallback has been removed
// to prevent timing attacks and enforce strong password storage.
// 企业为何需要：明文密码回退分支允许管理员密码以明文存储在数据库中，一旦数据库泄露即可直接使用。
// 强制 bcrypt 消除了这一攻击面。
func compareAdminPassword(plaintext, stored string) bool {
	if !isBcryptHash(stored) {
		return false // reject non-bcrypt hashes (legacy plaintext no longer supported)
	}
	err := bcrypt.CompareHashAndPassword([]byte(stored), []byte(plaintext))
	return err == nil
}

// hashAdminPassword hashes a password using bcrypt.
func hashAdminPassword(password string) (string, error) {
	bytes, err := bcryptGenerate([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// isBcryptHash checks if a string looks like a bcrypt hash.
func isBcryptHash(s string) bool {
	return len(s) == 60 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

// AuditPasswordChange records a password change in the audit log.
// Called from UpdateConfig when adminPassword is updated.
func AuditPasswordChange(ctx context.Context, actorIP string) {
	audit.Log(ctx, audit.AuditEntry{
		Action:    "admin.password.change",
		ActorID:   "admin",
		ActorIP:   actorIP,
		Resource:  "admin/config/global:admin_password",
		Before:    maskedKey,
		After:     maskedKey,
		RequestID: middleware.GetRequestID(ctx),
	})
}
