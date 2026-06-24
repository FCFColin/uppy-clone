package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/uppy-clone/backend/internal/apierror"
	"github.com/uppy-clone/backend/internal/auth"
	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
	"github.com/uppy-clone/backend/internal/protocol"
	"github.com/uppy-clone/backend/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// LobbyHandler handles lobby/room endpoints.
type LobbyHandler struct {
	hub            *game.Hub
	jwtMgr         *auth.JWTManager
	logger         *slog.Logger
	allowedOrigins []string
}

// NewLobbyHandler creates a new LobbyHandler.
func NewLobbyHandler(hub *game.Hub, jwtMgr *auth.JWTManager, allowedOrigins []string) *LobbyHandler {
	return &LobbyHandler{
		hub:            hub,
		jwtMgr:         jwtMgr,
		logger:         slog.Default().With("component", "lobby_handler"),
		allowedOrigins: allowedOrigins,
	}
}

// wsStaticSpanAttr is the pre-allocated static attribute shared by all WebSocket
// read/write pump spans. Per-span dynamic attributes (destination, player_id,
// message_type, message_size) are constructed only when a span is actually
// created, which — combined with ping/tap sampling — keeps allocations off the
// hot path.
var wsStaticSpanAttr = attribute.String("messaging.system", "websocket")

// CreateRoom handles POST /api/registry/create
func (h *LobbyHandler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		// Hub unavailable — return degraded response suggesting retry
		slog.Warn("degraded: Hub not available, cannot create room")
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]string{"code": ""},
			"Room service temporarily unavailable, please retry")
		return
	}

	code, err := h.hub.CreateRoom(r.Context())
	// 409 Conflict: 资源冲突（如房间代码重复）。企业为何需要：409 让客户端知道可以重试（换一个代码），而 500 意味着服务器内部错误。
	if err == game.ErrRoomCodeConflict {
		apierror.Conflict("Room code conflict, please retry").Write(w)
		return
	}
	if err != nil {
		// Hub returned an unexpected error — degraded response
		slog.Warn("degraded: Hub.CreateRoom failed", "error", err)
		WriteDegradedJSON(w, http.StatusServiceUnavailable,
			map[string]string{"code": ""},
			"Room creation temporarily unavailable, please retry")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}

// CheckRoom handles GET /api/registry/check/{code}
func (h *LobbyHandler) CheckRoom(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		apierror.BadRequest("Room code is required").Write(w)
		return
	}

	if len(code) != config.RoomCodeLen {
		apierror.BadRequest("invalid room code").Write(w)
		return
	}
	for _, c := range code {
		if (c < 'A' || c > 'Z') && (c < '2' || c > '9') {
			apierror.BadRequest("invalid room code charset").Write(w)
			return
		}
	}

	if h.hub == nil {
		// Hub unavailable — return degraded response with exists=false
		slog.Warn("degraded: Hub not available, cannot check room")
		WriteDegradedJSON(w, http.StatusOK,
			map[string]interface{}{
				"code":     code,
				"exists":   false,
				"degraded": true,
			},
			"Room check temporarily unavailable")
		return
	}

	info, err := h.hub.CheckRoom(code)
	if err != nil {
		// DB unavailable — return degraded response with exists=false
		slog.Warn("degraded: CheckRoom failed, returning not-found", "code", code, "error", err)
		WriteDegradedJSON(w, http.StatusOK,
			map[string]interface{}{
				"code":     code,
				"exists":   false,
				"degraded": true,
			},
			"Room check temporarily unavailable")
		return
	}

	if info == nil {
		apierror.NotFound("Room not found").Write(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	// Last-Modified: room creation timestamp as HTTP date (RFC 7232 weak validator).
	// Enables conditional GET via If-Modified-Since on the client side.
	w.Header().Set("Last-Modified", time.Unix(info.CreatedAt, 0).UTC().Format(http.TimeFormat))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":        info.Code,
		"phase":       info.Phase,
		"playerCount": info.PlayerCount,
		"createdAt":   info.CreatedAt,
	})
}

// writeDegradedLobbyList writes a degraded response when Redis/DB is unavailable.
// 企业为何需要：消除 ListLobbies 中两处重复的降级响应代码，集中维护响应格式。
func writeDegradedLobbyList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"lobbies":     []interface{}{},
		"total":       0,
		"has_more":    false,
		"next_cursor": "",
		"degraded":    true,
	})
}

// ListLobbies handles GET /api/registry/lobbies
// 企业为何需要：Offset 分页在深页（大 offset）时性能差，需扫描并丢弃前 N 行。
// Cursor 分页利用索引直接定位，性能恒定。这是"offset 分页有什么问题"的标准面试答案。
func (h *LobbyHandler) ListLobbies(w http.ResponseWriter, r *http.Request) {
	limit := config.DefaultPageSize
	cursor := r.URL.Query().Get("cursor")
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= config.MaxPageSize {
			limit = v
		}
	}

	result, err := h.hub.ListLobbies(r.Context(), limit, cursor)
	if err != nil {
		// Enterprise rationale: Graceful degradation — return empty list instead of 500
		// when DB is unavailable. Users can still create rooms; they just can't see
		// existing ones. Trade-off: stale data, but better than complete outage.
		slog.Warn("degraded: returning empty lobby list", "error", err)
		writeDegradedLobbyList(w)
		return
	}

	response := map[string]interface{}{
		"lobbies":     result.Lobbies,
		"total":       result.Total,
		"has_more":    result.HasMore,
		"next_cursor": result.NextCursor,
	}
	bodyBytes, err := json.Marshal(response)
	if err != nil {
		slog.Warn("ListLobbies: failed to marshal response", "error", err)
		writeDegradedLobbyList(w)
		return
	}

	// ETag conditional request (RFC 7232): compute a strong validator from the
	// response body. If the client's If-None-Match matches, return 304 Not Modified
	// to save bandwidth and avoid re-rendering on the client.
	// 企业为何需要：列表数据不频繁变化，ETag 让客户端缓存命中时免传 body，
	// 降低带宽与延迟。304 比 200+body 小几个数量级。
	hash := sha256.Sum256(bodyBytes)
	etag := fmt.Sprintf(`"%x"`, hash[:16])

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(bodyBytes)
}

// WebSocket handles GET /lobby/{code}/ws — WebSocket upgrade
// 企业为何需要：舱壁隔离（Bulkhead）防止单类资源耗尽拖垮整体。WebSocket 连接洪水可耗尽文件描述符和内存，
// 导致 REST API 也无法响应。连接上限是 DoS 防御的基本措施。
func (h *LobbyHandler) WebSocket(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		apierror.BadRequest("Room code is required").Write(w)
		return
	}

	userId, ok := h.authenticateWSRequest(w, r)
	if !ok {
		return
	}

	if !h.validateWSOrigin(w, r) {
		return
	}

	if !h.checkWSRateLimit(w, r) {
		return
	}

	room := h.hub.GetRoom(code)
	if room == nil {
		apierror.NotFound("Room not found").Write(w)
		return
	}

	conn, ok := h.upgradeWSConnection(w, r)
	if !ok {
		return
	}

	h.startWSPumps(room, userId, conn, r.Context())
}

// authenticateWSRequest authenticates the WebSocket request via cookie (session or quickplay).
func (h *LobbyHandler) authenticateWSRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	userId, _, ok := auth.GetAuthenticatedUser(r)
	if !ok || userId == "" {
		apierror.Unauthorized("Unauthorized").Write(w)
		return "", false
	}
	return userId, true
}

// validateWSOrigin validates the Origin header against allowed origins (CSWSH protection).
// 企业为何需要：Cross-Site WebSocket Hijacking (CSWSH) 允许恶意页面建立 WebSocket 连接
// 携带受害者 cookie。强制校验 Origin 是防御 CSWSH 的标准措施。空 Origin 必须拒绝。
func (h *LobbyHandler) validateWSOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		apierror.Forbidden("origin required").Write(w)
		return false
	}
	originURL, err := url.Parse(origin)
	if err != nil {
		apierror.Forbidden("invalid origin").Write(w)
		return false
	}
	originHost := originURL.Hostname()
	allowed := false
	for _, ao := range h.allowedOrigins {
		aoURL, _ := url.Parse(ao)
		if aoURL != nil && aoURL.Hostname() == originHost {
			allowed = true
			break
		}
	}
	if !allowed {
		h.logger.Warn("CSWSH blocked", "origin", origin)
		apierror.Forbidden("origin not allowed").Write(w)
		return false
	}
	return true
}

// checkWSRateLimit checks the global WebSocket connection limit (bulkhead isolation).
func (h *LobbyHandler) checkWSRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if !h.hub.CanAcceptWSConnection() {
		apierror.New(http.StatusServiceUnavailable, "Service Unavailable",
			"WebSocket connection limit reached, please try again later").Write(w)
		return false
	}
	return true
}

// upgradeWSConnection upgrades the HTTP connection to WebSocket.
// Origin is already validated manually in validateWSOrigin; a per-request upgrader
// with CheckOrigin=true is used to avoid mutating the shared package-level upgrader.
func (h *LobbyHandler) upgradeWSConnection(w http.ResponseWriter, r *http.Request) (*websocket.Conn, bool) {
	reqUpgrader := websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
	conn, err := reqUpgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("websocket upgrade failed", "error", err)
		return nil, false
	}
	return conn, true
}

// startWSPumps increments the connection counter, handles join, and starts read/write pumps.
// 企业为何需要：从 HTTP 请求上下文派生可取消上下文，使 WS 连接生命周期内的所有 span
// 形成父子关系（parent-child），便于分布式追踪。readPump 退出（连接关闭）时调用 cancel()
// 释放资源并使 writePump 感知连接结束。
func (h *LobbyHandler) startWSPumps(room *game.Room, userId string, conn *websocket.Conn, reqCtx context.Context) {
	h.hub.IncrementWSConnection()

	if err := room.HandleJoin(userId, conn); err != nil {
		h.logger.Error("handle join failed", "error", err)
		h.hub.DecrementWSConnection()
		_ = conn.Close()
		return
	}

	// Derive a cancellable context from the HTTP request context.
	// WS 升级后 r.Context() 会在 handler 返回时取消，因此创建独立的可取消上下文，
	// 生命周期绑定到 WS 连接。readPump 退出时调用 cancel()。
	wsCtx, cancel := context.WithCancel(reqCtx)
	go h.writePump(room, userId, conn, wsCtx)
	h.readPump(room, userId, conn, wsCtx, cancel)
}

// readPump reads messages from the WebSocket and routes them to the room.
// wsCtx 是从 HTTP 请求上下文派生的可取消上下文，用于创建父子 span；连接关闭时调用 cancel()。
func (h *LobbyHandler) readPump(room *game.Room, playerID string, conn *websocket.Conn, wsCtx context.Context, cancel context.CancelFunc) {
	defer func() {
		cancel() // 取消 WS 上下文，使 writePump 及派生 span 感知连接结束
		_ = conn.Close()
		_ = room.HandleDisconnect(playerID)
		h.hub.DecrementWSConnection()
	}()

	conn.SetReadLimit(config.WSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(h.hub.Timeouts().WSPongTimeout))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(h.hub.Timeouts().WSPongTimeout))
		return nil
	})

	// tapSpanCounter counts tap messages for 1% span sampling.
	var tapSpanCounter uint64

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Warn("read error", "playerID", playerID, "error", err)
			}
			break
		}

		if len(message) == 0 {
			continue
		}

		msgType, payload := protocol.DecodeMessage(message)

		// Hot-path optimization: skip span creation for high-frequency heartbeats
		// (MsgPing) and sample MsgTap spans at 1% to reduce tracing overhead.
		// MsgPing and MsgTap dominate the message volume, so excluding them from
		// 100% tracing dramatically cuts span allocation pressure.
		var span trace.Span
		createSpan := true
		switch msgType {
		case protocol.MsgPing:
			createSpan = false
		case protocol.MsgTap:
			tapSpanCounter++
			if tapSpanCounter%100 != 0 {
				createSpan = false
			}
		}

		if createSpan {
			var msgTypeName string
			switch msgType {
			case protocol.MsgTap:
				msgTypeName = "tap"
			case protocol.MsgSetNickname:
				msgTypeName = "set_nickname"
			case protocol.MsgRestartVote:
				msgTypeName = "restart_vote"
			case protocol.MsgPing:
				msgTypeName = "ping"
			default:
				msgTypeName = "unknown"
			}
			// 从 wsCtx 派生 span，建立父子关系（parent-child），
			// 使 WS 消息处理 span 关联到 HTTP 升级请求的 trace 链路。
			_, span = telemetry.Tracer().Start(wsCtx, "ws.readPump."+msgTypeName,
				trace.WithAttributes(
					wsStaticSpanAttr,
					attribute.String("messaging.destination", room.Code()),
					attribute.String("messaging.message_type", msgTypeName),
					attribute.String("messaging.player_id", playerID),
				),
			)
		}
		if err := room.HandleMessage(playerID, msgType, payload); err != nil {
			if span != nil {
				span.RecordError(err)
			}
			h.logger.Error("handle message error", "playerID", playerID, "error", err)
		}
		if span != nil {
			span.End()
		}
	}
}

// writePump reads from the player's Send channel and writes to the WebSocket.
// wsCtx 是从 HTTP 请求上下文派生的可取消上下文，用于创建父子 span。
func (h *LobbyHandler) writePump(room *game.Room, playerID string, conn *websocket.Conn, wsCtx context.Context) {
	pc := room.GetConnection(playerID)
	if pc == nil {
		return
	}

	ticker := time.NewTicker(h.hub.Timeouts().WSPingInterval)
	defer func() {
		ticker.Stop()
		conn.Close()
	}()

	for {
		select {
		case msg, ok := <-pc.Send:
			_ = conn.SetWriteDeadline(time.Now().Add(h.hub.Timeouts().WSWriteTimeout))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// 从 wsCtx 派生 span，建立父子关系（parent-child），
			// 使 WS 广播 span 关联到 HTTP 升级请求的 trace 链路。
			_, span := telemetry.Tracer().Start(wsCtx, "ws.writePump.broadcast",
				trace.WithAttributes(
					wsStaticSpanAttr,
					attribute.String("messaging.destination", room.Code()),
					attribute.String("messaging.player_id", playerID),
					attribute.Int("messaging.message_size", len(msg)),
				),
			)
			if err := conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				span.RecordError(err)
				span.End()
				return
			}
			span.End()

		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(h.hub.Timeouts().WSWriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
