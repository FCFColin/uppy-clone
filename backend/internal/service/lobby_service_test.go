package service

// 企业为何需要：LobbyService 是大厅业务逻辑的薄封装层。
// 本测试覆盖：
// - 构造器正确存储 Hub 引用
// - Hub() getter 返回正确的引用
// - nil hub 传入和 nil receiver 防御

import (
	"testing"

	"github.com/uppy-clone/backend/internal/config"
	"github.com/uppy-clone/backend/internal/game"
)

// newTestHub 创建一个用于测试的 Hub 实例（依赖均为 nil，不连接真实 DB/Redis）。
func newTestHub() *game.Hub {
	return game.NewHub(nil, nil, config.TimeoutConfig{}, 0, 0, nil)
}

// --- NewLobbyService ---

func TestNewLobbyService(t *testing.T) {
	hub := newTestHub()
	svc := NewLobbyService(hub)

	if svc == nil {
		t.Fatal("NewLobbyService 不应返回 nil")
	}
	if svc.hub != hub {
		t.Error("构造器应存储 hub 引用")
	}
}

func TestNewLobbyService_NilHub(t *testing.T) {
	svc := NewLobbyService(nil)
	if svc == nil {
		t.Fatal("NewLobbyService 不应返回 nil")
	}
	if svc.hub != nil {
		t.Error("hub 应为 nil")
	}
}

// --- Hub() ---

func TestLobbyService_Hub_ReturnsHub(t *testing.T) {
	hub := newTestHub()
	svc := NewLobbyService(hub)

	got := svc.Hub()
	if got != hub {
		t.Error("Hub() 应返回构造器传入的 hub 引用")
	}
}

func TestLobbyService_Hub_NilHub(t *testing.T) {
	svc := NewLobbyService(nil)
	if svc.Hub() != nil {
		t.Error("nil hub 传入时 Hub() 应返回 nil")
	}
}

func TestLobbyService_Hub_NilReceiver(t *testing.T) {
	// nil receiver 调用 Hub() 应 panic（访问 s.hub 字段触发空指针解引用）。
	var svc *LobbyService

	assertPanics(t, "nil receiver Hub()", func() {
		_ = svc.Hub()
	})
}

// 表驱动测试：Hub() 在不同构造参数下的行为。
func TestLobbyService_Hub_TableDriven(t *testing.T) {
	hub := newTestHub()
	tests := []struct {
		name    string
		hub     *game.Hub
		wantNil bool
	}{
		{"非 nil hub", hub, false},
		{"nil hub", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewLobbyService(tt.hub)
			got := svc.Hub()
			if tt.wantNil {
				if got != nil {
					t.Error("期望 nil hub，得到非 nil")
				}
			} else {
				if got != tt.hub {
					t.Error("Hub() 应返回构造器传入的 hub")
				}
			}
		})
	}
}
