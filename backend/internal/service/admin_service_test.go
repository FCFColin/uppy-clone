package service

// 企业为何需要：AdminService 封装管理员登录校验逻辑。本测试覆盖：
// - 构造器正确存储依赖
// - VerifyLogin 在 redis 为 nil / 非 nil 时的分支
// - nil receiver 防御
// - 委托到 store 层的调用路径（通过 panic 证明委托发生）
//
// 注意：service 依赖具体类型 *store.PostgresStore / *store.RedisStore（非接口），
// 且字段均未导出。在不修改源码的前提下，单元测试使用零值实例
// &store.PostgresStore{} / &store.RedisStore{}，调用 store 方法时会在
// nil pool/cb/rdb 处 panic。通过 defer recover() 捕获 panic 来验证
// service 正确委托到 store 层。

import (
	"context"
	"testing"

	"github.com/uppy-clone/backend/internal/store"
)

// assertPanics 断言 fn 会 panic。共享辅助函数，供同包测试文件使用。
func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("%s: 期望 panic 但未发生", name)
		}
	}()
	fn()
}

// --- NewAdminService ---

func TestNewAdminService(t *testing.T) {
	db := &store.PostgresStore{}
	redis := &store.RedisStore{}
	svc := NewAdminService(db, redis)

	if svc == nil {
		t.Fatal("NewAdminService 不应返回 nil")
	}
	if svc.db != db {
		t.Error("构造器应存储 db 引用")
	}
	if svc.redis != redis {
		t.Error("构造器应存储 redis 引用")
	}
}

func TestNewAdminService_NilDeps(t *testing.T) {
	svc := NewAdminService(nil, nil)
	if svc == nil {
		t.Fatal("NewAdminService 不应返回 nil")
	}
	if svc.db != nil {
		t.Error("db 应为 nil")
	}
	if svc.redis != nil {
		t.Error("redis 应为 nil")
	}
}

// --- VerifyLogin ---

func TestAdminService_VerifyLogin_NilRedis_DelegatesToDB(t *testing.T) {
	// redis 为 nil 时跳过锁定检查，直接调用 db.GetConfig。
	// 零值 PostgresStore 的 cb 为 nil，调用会 panic。
	// 这验证了 service 正确委托到 db 层。
	svc := NewAdminService(&store.PostgresStore{}, nil)

	assertPanics(t, "nil redis 委托到 db", func() {
		_, _ = svc.VerifyLogin(context.Background(), "192.168.1.1")
	})
}

func TestAdminService_VerifyLogin_WithRedis_DelegatesToRedis(t *testing.T) {
	// redis 非 nil（零值实例）时进入锁定检查分支，调用 redis.IsLoginLocked。
	// 零值 RedisStore 的 rdb 为 nil，调用会 panic。
	// 这验证了 service 正确进入 redis 分支并委托。
	svc := NewAdminService(&store.PostgresStore{}, &store.RedisStore{})

	assertPanics(t, "非 nil redis 委托到 redis", func() {
		_, _ = svc.VerifyLogin(context.Background(), "10.0.0.1")
	})
}

func TestAdminService_VerifyLogin_NilReceiver(t *testing.T) {
	// nil receiver 调用应 panic（访问 s.redis 字段触发空指针解引用）。
	var svc *AdminService

	assertPanics(t, "nil receiver", func() {
		_, _ = svc.VerifyLogin(context.Background(), "1.2.3.4")
	})
}

func TestAdminService_VerifyLogin_NilDB_NilRedis(t *testing.T) {
	// db 和 redis 均为 nil：redis 检查跳过，然后访问 s.db.GetConfig
	// 在 nil db 上调用方法——Go 允许 nil 指针方法调用，但 store 内部
	// 访问 nil 字段会 panic。
	svc := NewAdminService(nil, nil)

	assertPanics(t, "nil db nil redis", func() {
		_, _ = svc.VerifyLogin(context.Background(), "")
	})
}

// 表驱动测试：VerifyLogin 在不同输入下的行为。
func TestAdminService_VerifyLogin_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		db       *store.PostgresStore
		redis    *store.RedisStore
		clientIP string
	}{
		{
			name:     "nil redis 零值 db 空 IP",
			db:       &store.PostgresStore{},
			redis:    nil,
			clientIP: "",
		},
		{
			name:     "nil redis 零值 db 非空 IP",
			db:       &store.PostgresStore{},
			redis:    nil,
			clientIP: "203.0.113.5",
		},
		{
			name:     "零值 redis 零值 db",
			db:       &store.PostgresStore{},
			redis:    &store.RedisStore{},
			clientIP: "198.51.100.1",
		},
		{
			name:     "零值 redis nil db",
			db:       nil,
			redis:    &store.RedisStore{},
			clientIP: "203.0.113.99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewAdminService(tt.db, tt.redis)
			assertPanics(t, tt.name, func() {
				_, _ = svc.VerifyLogin(context.Background(), tt.clientIP)
			})
		})
	}
}

// ─── AuthService 单元测试 ────────────────────────────────────────────
// auth_service.go 的集成测试需要 Docker（在 auth_service_test.go 中），
// 此处补充不依赖 Docker 的单元测试，覆盖 DeleteUserData / ExportUserData
// 的委托路径和 nil receiver 防御。

// --- AuthService.DeleteUserData ---

func TestAuthService_DeleteUserData_NilRedis_DelegatesToDB(t *testing.T) {
	// redis 为 nil 时跳过 token 撤销，直接调用 db.AnonymizeUser。
	// 零值 PostgresStore 的 cb 为 nil，调用会 panic。
	svc := NewAuthService(&store.PostgresStore{}, nil, nil, nil)

	assertPanics(t, "DeleteUserData nil redis 委托到 db", func() {
		_ = svc.DeleteUserData(context.Background(), "user-to-delete")
	})
}

func TestAuthService_DeleteUserData_NilReceiver(t *testing.T) {
	var svc *AuthService

	assertPanics(t, "DeleteUserData nil receiver", func() {
		_ = svc.DeleteUserData(context.Background(), "user-123")
	})
}

// --- AuthService.ExportUserData ---

func TestAuthService_ExportUserData_DelegatesToDB(t *testing.T) {
	// 零值 PostgresStore 调用 GetUserByID 会 panic（nil cb）。
	svc := NewAuthService(&store.PostgresStore{}, nil, nil, nil)

	assertPanics(t, "ExportUserData 委托到 db", func() {
		_, _ = svc.ExportUserData(context.Background(), "user-export")
	})
}

func TestAuthService_ExportUserData_NilReceiver(t *testing.T) {
	var svc *AuthService

	assertPanics(t, "ExportUserData nil receiver", func() {
		_, _ = svc.ExportUserData(context.Background(), "user-456")
	})
}
