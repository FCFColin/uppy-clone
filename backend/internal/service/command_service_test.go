package service

// 企业为何需要：CommandService 是 CQRS 写路径的薄委托层。
// 本测试覆盖：
// - 构造器正确存储 db 引用
// - CreateUser / AnonymizeUser 正确委托到 db 层
// - nil receiver 防御
// - 表驱动测试覆盖多种输入组合
//
// 测试策略：service 依赖具体类型 *store.PostgresStore（非接口），字段未导出。
// 使用零值 &store.PostgresStore{} 调用委托方法时，store 内部 nil cb 会 panic。
// 通过 defer recover() 捕获 panic 来验证 service 正确委托到 store。

import (
	"context"
	"testing"

	"github.com/uppy-clone/backend/internal/domain"
	"github.com/uppy-clone/backend/internal/store"
)

// --- NewCommandService ---

func TestNewCommandService(t *testing.T) {
	db := &store.PostgresStore{}
	svc := NewCommandService(db)

	if svc == nil {
		t.Fatal("NewCommandService 不应返回 nil")
	}
	if svc.db != db {
		t.Error("构造器应存储 db 引用")
	}
}

func TestNewCommandService_NilDB(t *testing.T) {
	svc := NewCommandService(nil)
	if svc == nil {
		t.Fatal("NewCommandService 不应返回 nil")
	}
	if svc.db != nil {
		t.Error("db 应为 nil")
	}
}

// --- CreateUser ---

func TestCommandService_CreateUser_DelegatesToDB(t *testing.T) {
	// 零值 PostgresStore 的 cb 为 nil，CreateUser 内部调用 s.cb.Execute 会 panic。
	// 这验证了 service 正确委托到 db.CreateUser。
	svc := NewCommandService(&store.PostgresStore{})
	user := &domain.User{
		ID:        "user-1",
		Email:     "test@example.com",
		Nickname:  "TestUser",
		Palette:   0,
		CreatedAt: 1700000000,
	}

	assertPanics(t, "CreateUser 委托到 db", func() {
		_ = svc.CreateUser(context.Background(), user)
	})
}

func TestCommandService_CreateUser_NilDB(t *testing.T) {
	// db 为 nil 时，c.db.CreateUser 在 nil 上调用方法。
	// Go 允许 nil 指针方法调用，但 store 内部访问 nil 字段会 panic。
	svc := NewCommandService(nil)
	user := &domain.User{ID: "user-2"}

	assertPanics(t, "CreateUser nil db", func() {
		_ = svc.CreateUser(context.Background(), user)
	})
}

func TestCommandService_CreateUser_NilUser(t *testing.T) {
	// 传入 nil user 也应委托到 db（store 层负责校验）。
	svc := NewCommandService(&store.PostgresStore{})

	assertPanics(t, "CreateUser nil user", func() {
		_ = svc.CreateUser(context.Background(), nil)
	})
}

func TestCommandService_CreateUser_NilReceiver(t *testing.T) {
	// nil receiver 调用应 panic（访问 c.db 字段触发空指针解引用）。
	var svc *CommandService

	assertPanics(t, "CreateUser nil receiver", func() {
		_ = svc.CreateUser(context.Background(), &domain.User{})
	})
}

// --- AnonymizeUser ---

func TestCommandService_AnonymizeUser_DelegatesToDB(t *testing.T) {
	// 零值 PostgresStore 调用 AnonymizeUser 会 panic（nil cb）。
	// 这验证了 service 正确委托到 db.AnonymizeUser。
	svc := NewCommandService(&store.PostgresStore{})

	assertPanics(t, "AnonymizeUser 委托到 db", func() {
		_ = svc.AnonymizeUser(context.Background(), "user-to-anonymize")
	})
}

func TestCommandService_AnonymizeUser_NilDB(t *testing.T) {
	svc := NewCommandService(nil)

	assertPanics(t, "AnonymizeUser nil db", func() {
		_ = svc.AnonymizeUser(context.Background(), "user-123")
	})
}

func TestCommandService_AnonymizeUser_EmptyUserID(t *testing.T) {
	// 空字符串 userID 也应委托到 db（store 层负责处理）。
	svc := NewCommandService(&store.PostgresStore{})

	assertPanics(t, "AnonymizeUser 空 userID", func() {
		_ = svc.AnonymizeUser(context.Background(), "")
	})
}

func TestCommandService_AnonymizeUser_NilReceiver(t *testing.T) {
	var svc *CommandService

	assertPanics(t, "AnonymizeUser nil receiver", func() {
		_ = svc.AnonymizeUser(context.Background(), "user-456")
	})
}

// --- 表驱动测试 ---

func TestCommandService_CreateUser_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		db   *store.PostgresStore
		user *domain.User
	}{
		{
			name: "零值 db 完整 user",
			db:   &store.PostgresStore{},
			user: &domain.User{
				ID:        "u1",
				Email:     "a@b.com",
				Nickname:  "Alice",
				Palette:   1,
				CreatedAt: 1700000000,
			},
		},
		{
			name: "零值 db 最小 user",
			db:   &store.PostgresStore{},
			user: &domain.User{ID: "u2"},
		},
		{
			name: "nil db",
			db:   nil,
			user: &domain.User{ID: "u3"},
		},
		{
			name: "零值 db nil user",
			db:   &store.PostgresStore{},
			user: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewCommandService(tt.db)
			assertPanics(t, tt.name, func() {
				_ = svc.CreateUser(context.Background(), tt.user)
			})
		})
	}
}

func TestCommandService_AnonymizeUser_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		db     *store.PostgresStore
		userID string
	}{
		{"零值 db 正常 ID", &store.PostgresStore{}, "user-abc"},
		{"零值 db 空 ID", &store.PostgresStore{}, ""},
		{"nil db 正常 ID", nil, "user-xyz"},
		{"nil db 空 ID", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewCommandService(tt.db)
			assertPanics(t, tt.name, func() {
				_ = svc.AnonymizeUser(context.Background(), tt.userID)
			})
		})
	}
}
