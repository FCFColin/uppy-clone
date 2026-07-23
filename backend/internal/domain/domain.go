package domain

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"

	"github.com/uppy-clone/backend/internal/config"
)

const (
	MaxScore            = 9999
	ReconnectGraceMs    = 30000
	RestartTimeoutMs    = 30000
	AutoRestartMs       = 60000
	MaxNicknameLen      = 12
	NicknameCooldownMs  = 30000
	MessageRateLimit    = 100
)

var ErrDuplicateUser = errors.New("duplicate user")
var ErrNotFound = errors.New("resource not found")
var ErrValidation = errors.New("validation failed")

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

const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// WithRole returns a new context with the given role value.
// 角色 must come from verified credentials (JWT claims/authenticated middleware),
// not client-controllable HTTP headers. X-User-Role can be forged by any client,
// leading to privilege escalation. This function is called by auth middleware
// after verifying credentials; RBAC middleware then reads the role from context.
func WithRole(ctx context.Context, role string) context.Context {
	return ContextKeyRole.WithValue(ctx, role)
}

func RoleFromContext(ctx context.Context) (string, bool) {
	return ContextKeyRole.Value(ctx)
}

type proxyKey struct{}

func WithTrustedProxy(ctx context.Context, trusted bool) context.Context {
	return context.WithValue(ctx, proxyKey{}, trusted)
}

func IsTrustedProxy(ctx context.Context) bool {
	v, ok := ctx.Value(proxyKey{}).(bool)
	return ok && v
}

// ProblemDetails represents an RFC 7807 problem details response.
type ProblemDetails struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Instance string `json:"instance,omitempty"`
}

func New(status int, title, detail string) *ProblemDetails {
	if status < 100 || status >= 600 {
		status = http.StatusInternalServerError
		title = "Internal Server Error"
	}
	return &ProblemDetails{
		Type:   fmt.Sprintf("https://httpstatuses.com/%d", status),
		Title:  title,
		Status: status,
		Detail: detail,
	}
}

func BadRequest(detail string) *ProblemDetails {
	return New(http.StatusBadRequest, "Bad Request", detail)
}

func Unauthorized(detail string) *ProblemDetails {
	return New(http.StatusUnauthorized, "Unauthorized", detail)
}

func Forbidden(detail string) *ProblemDetails {
	return New(http.StatusForbidden, "Forbidden", detail)
}

func NotFound(detail string) *ProblemDetails {
	return New(http.StatusNotFound, "Not Found", detail)
}

func Conflict(detail string) *ProblemDetails {
	return New(http.StatusConflict, "Conflict", detail)
}

func UnprocessableEntity(detail string) *ProblemDetails {
	return New(http.StatusUnprocessableEntity, "Unprocessable Entity", detail)
}

func TooManyRequests(detail string) *ProblemDetails {
	return New(http.StatusTooManyRequests, "Too Many Requests", detail)
}

func InternalError(detail string) *ProblemDetails {
	return New(http.StatusInternalServerError, "Internal Server Error", detail)
}

func ServiceUnavailable(detail string) *ProblemDetails {
	return New(http.StatusServiceUnavailable, "Service Unavailable", detail)
}

func (e *ProblemDetails) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	if err := json.NewEncoder(w).Encode(e); err != nil {
		slog.Debug("problem-details write failed", "err", err)
	}
}

// rngSource is the minimal RNG interface needed for room code generation.
type rngSource interface {
	IntN(n int) int
}

type seededRNG struct {
	rng *rand.Rand
}

func (s *seededRNG) IntN(n int) int { return s.rng.IntN(n) }

func NewSeededRNG(seed int64) *seededRNG {
	return &seededRNG{rng: rand.New(rand.NewPCG(uint64(seed), uint64(seed^0xDEADBEEF)))} //nolint:gosec // G404: game RNG, not crypto
}

const roomAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // pragma: allowlist secret

func GenerateRoomCode(rng rngSource) string {
	code := make([]byte, config.RoomCodeLen)
	for i := range code {
		code[i] = roomAlphabet[rng.IntN(len(roomAlphabet))]
	}
	return string(code)
}

type RoomCodeGenerator struct {
	rng   rngSource
	mu    sync.Mutex
	genFn func() string
}

func NewRoomCodeGenerator(seed int64) *RoomCodeGenerator {
	g := &RoomCodeGenerator{
		rng: NewSeededRNG(seed),
	}
	g.genFn = func() string {
		return GenerateRoomCode(g.rng)
	}
	return g
}

// SetGenerateRoomCodeHook overrides room code generation and returns a
// restore function to revert to the original behavior.
func (g *RoomCodeGenerator) SetGenerateRoomCodeHook(fn func() string) (restore func()) {
	g.mu.Lock()
	orig := g.genFn
	g.genFn = fn
	g.mu.Unlock()
	return func() {
		g.mu.Lock()
		g.genFn = orig
		g.mu.Unlock()
	}
}

func (g *RoomCodeGenerator) GenerateRoomCode() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.genFn()
}

func UUID() string {
	b := make([]byte, 16)
	if _, err := crand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
