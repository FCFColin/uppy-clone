package domain

import "context"

type ContextKey string

const (
	ContextKeyUserID   ContextKey = "auth_user_id"
	ContextKeyNickname ContextKey = "auth_nickname"
	ContextKeyRole     ContextKey = "auth_user_role"
	ContextKeyJTI      ContextKey = "auth_jti"
)

func (k ContextKey) WithValue(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, k, v)
}

func (k ContextKey) Value(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(k).(string)
	return v, ok
}
