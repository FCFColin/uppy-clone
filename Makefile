.PHONY: dev test lint build run migrate seed bench audit clean

# 一键启动：PostgreSQL + Redis + 后端热重载 + 前端
dev:
	docker compose up -d postgres redis
	cd backend && air &
	cd frontend && npm run dev

# 运行所有测试
test:
	cd backend && go test -race ./...
	cd frontend && npm test

# Lint 检查
lint:
	cd backend && golangci-lint run
	cd frontend && npm run lint

# 构建生产产物
build:
	cd backend && go build -o bin/server ./cmd/server
	cd frontend && npm run build

# 运行服务器
run:
	cd backend && go run ./cmd/server

# 数据库迁移
migrate:
	cd backend && go run ./cmd/server -migrate

# 插入测试数据
seed:
	cd backend && go run ./cmd/seed

# 基准测试
bench:
	cd backend && go test -bench=. ./... | tee docs/benchmarks-v2.md

# 安全审计
audit:
	cd backend && govulncheck ./...
	gitleaks detect --source . --report-path leaks.json
	trivy fs .

# 清理
clean:
	rm -rf backend/bin frontend/dist
	docker compose down -v
