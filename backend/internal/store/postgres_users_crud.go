package store

import (
	"context"

	"github.com/uppy-clone/backend/internal/audit"
	"github.com/uppy-clone/backend/internal/domain"
)

func logUserCreateAudit(ctx context.Context, u *domain.User) {
	audit.Log(ctx, audit.AuditEntry{
		Action:   "user.create",
		ActorID:  u.ID,
		Resource: "user/" + u.ID,
		After: map[string]interface{}{
			"id":       u.ID,
			"nickname": u.Nickname,
		},
	})
}