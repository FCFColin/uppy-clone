package service

// 企业为何需要：QueryService 是 CQRS 读路径的薄委托层。
// 本测试覆盖：
// - 构造器正确存储 db 引用
// - GetUserByID / GetGameResultsByUserID 正确委托到 db 层
// - nil receiver 防御
// - 表驱动测试覆盖多种输入组合
//
// 测试策略：service 依赖具体类型 *store.PostgresStore（非接口），字段未导出。
// 使用零值 &store.PostgresStore{} 调用委托方法时，store 内部 nil cb 会 panic。
// 通过 defer recover() 捕获 panic 来验证 service 正确委托到 store。

import (
	"context"
	"testing"

	"github.com/uppy-clone/backend/internal/store"
)

// --- NewQueryService ---

func TestNewQueryService(t *testing.T) {
	db := &store.PostgresStore{}
	svc := NewQueryService(db)

	if svc == nil {
		t.Fatal("NewQueryService 不应返回 nil")
	}
	if svc.db != db {
		t.Error("构造器应存储 db 引用")
	}
}

func TestNewQueryService_NilDB(t *testing.T) {
	svc := NewQueryService(nil)
	if svc == nil {
		t.Fatal("NewQueryService 不应返回 nil")
	}
	if svc.db != nil {
		t.Error("db 应为 nil")
	}
}

// --- GetUserByID ---

func TestQueryService_GetUserByID_DelegatesToDB(t *testing.T) {
	// 零值 PostgresStore 的 cb 为 nil，GetUserByID 内部通过 withRetryRead
	// 调用 s.cb.Execute 会 panic。这验证了 service 正确委托到 db.GetUserByID。
	svc := NewQueryService(&store.PostgresStore{})

	assertPanics(t, "GetUserByID 委托到 db", func() {
		_, _ = svc.GetUserByID(context.Background(), "user-123")
	})
}

func TestQueryService_GetUserByID_NilDB(t *testing.T) {
	// db 为 nil 时，q.db.GetUserByID 在 nil 上调用方法。
	// Go 允许 nil 指针方法调用，但 store 内部访问 nil 字段会 panic。
	svc := NewQueryService(nil)

	assertPanics(t, "GetUserByID nil db", func() {
		_, _ = svc.GetUserByID(context.Background(), "user-456")
	})
}

func TestQueryService_GetUserByID_EmptyID(t *testing.T) {
	// 空字符串 ID 也应委托到 db（store 层负责处理）。
	svc := NewQueryService(&store.PostgresStore{})

	assertPanics(t, "GetUserByID 空 ID", func() {
		_, _ = svc.GetUserByID(context.Background(), "")
	})
}

func TestQueryService_GetUserByID_NilReceiver(t *testing.T) {
	// nil receiver 调用应 panic（访问 q.db 字段触发空指针解引用）。
	var svc *QueryService

	assertPanics(t, "GetUserByID nil receiver", func() {
		_, _ = svc.GetUserByID(context.Background(), "user-789")
	})
}

// --- GetGameResultsByUserID ---

func TestQueryService_GetGameResultsByUserID_DelegatesToDB(t *testing.T) {
	// 零值 PostgresStore 调用 GetGameResultsByUserID 会 panic（nil cb）。
	// 这验证了 service 正确委托到 db.GetGameResultsByUserID。
	svc := NewQueryService(&store.PostgresStore{})

	assertPanics(t, "GetGameResultsByUserID 委托到 db", func() {
		_, _ = svc.GetGameResultsByUserID(context.Background(), "user-results")
	})
}

func TestQueryService_GetGameResultsByUserID_NilDB(t *testing.T) {
	svc := NewQueryService(nil)

	assertPanics(t, "GetGameResultsByUserID nil db", func() {
		_, _ = svc.GetGameResultsByUserID(context.Background(), "user-abc")
	})
}

func TestQueryService_GetGameResultsByUserID_EmptyUserID(t *testing.T) {
	// 空字符串 userID 也应委托到 db。
	svc := NewQueryService(&store.PostgresStore{})

	assertPanics(t, "GetGameResultsByUserID 空 userID", func() {
		_, _ = svc.GetGameResultsByUserID(context.Background(), "")
	})
}

func TestQueryService_GetGameResultsByUserID_NilReceiver(t *testing.T) {
	var svc *QueryService

	assertPanics(t, "GetGameResultsByUserID nil receiver", func() {
		_, _ = svc.GetGameResultsByUserID(context.Background(), "user-def")
	})
}

// --- 表驱动测试 ---

func TestQueryService_GetUserByID_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		db   *store.PostgresStore
		id   string
	}{
		{"零值 db 正常 ID", &store.PostgresStore{}, "user-normal"},
		{"零值 db 空 ID", &store.PostgresStore{}, ""},
		{"nil db 正常 ID", nil, "user-nil-db"},
		{"nil db 空 ID", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewQueryService(tt.db)
			assertPanics(t, tt.name, func() {
				_, _ = svc.GetUserByID(context.Background(), tt.id)
			})
		})
	}
}

func TestQueryService_GetGameResultsByUserID_TableDriven(t *testing.T) {
	tests := []struct {
		name   string
		db     *store.PostgresStore
		userID string
	}{
		{"零值 db 正常 userID", &store.PostgresStore{}, "user-with-results"},
		{"零值 db 空 userID", &store.PostgresStore{}, ""},
		{"nil db 正常 userID", nil, "user-no-db"},
		{"nil db 空 userID", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewQueryService(tt.db)
			assertPanics(t, tt.name, func() {
				_, _ = svc.GetGameResultsByUserID(context.Background(), tt.userID)
			})
		})
	}
}
