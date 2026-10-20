.PHONY: dev dev-api dev-web dev-mock build build-web test lint clean smoke fake-gh

BIN := bin/graycrow
# 仅供本地开发；生产请用环境变量或 .env 注入真实密钥
MASTER_KEY ?= dev-only-master-key-please-change-0123456789

dev:
	@echo "API: http://localhost:8080   Web: http://localhost:5173（真实后端）"
	@MASTER_KEY=$(MASTER_KEY) go run ./cmd/graycrow & \
	cd web && bun dev; \
	kill %1 2>/dev/null || true

dev-mock:
	@echo "Web: http://localhost:5173（mock 数据，不依赖后端）"
	cd web && VITE_USE_MOCK=1 bun dev

dev-api:
	MASTER_KEY=$(MASTER_KEY) go run ./cmd/graycrow

dev-web:
	cd web && bun dev

build-web:
	cd web && bun run build

build: build-web
	CGO_ENABLED=0 go build -ldflags "-s -w" -o $(BIN) ./cmd/graycrow
	@echo "→ $(BIN)"

test:
	go test ./...

lint:
	go vet ./...
	cd web && node scripts/check-i18n.mjs && bun run build

smoke: build
	scripts/smoke.sh sqlite /tmp/graycrow-smoke.db

# 本地演示：假 GitHub（另开一个终端跑 make dev-api，并设 GITHUB_API_BASE=http://127.0.0.1:18081）
fake-gh:
	go run ./cmd/fakegithub

clean:
	rm -rf bin web/dist
