# Detect OS for binary extension
ifeq ($(OS),Windows_NT)
    BINARY_EXT := .exe
else
    BINARY_EXT :=
endif

BINARY  := modbus_client$(BINARY_EXT)
MODULE  := $(shell go list -m 2>/dev/null)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(DATE)

BUILD_FLAGS := -trimpath -ldflags "$(LDFLAGS)"
GO := go

.DEFAULT_GOAL := help

.PHONY: all build build-frontend build-debug run dev dev-frontend dev-backend \
        emulate test test-v fmt vet check deps deps-upgrade clean clean-logs install help info

# ─── Help ─────────────────────────────────────────────────────────────────────
help: ## Show this help message
	@printf '\n\033[1;32m ROC Modbus Expert — EPM\033[0m  \033[0;90m$(VERSION) ($(COMMIT))\033[0m\n\n'
	@printf ' \033[1mUsage:\033[0m  make \033[36m<target>\033[0m\n\n'
	@printf ' \033[1mTargets:\033[0m\n'
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "   \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf '\n'

# ─── Frontend ─────────────────────────────────────────────────────────────────
build-frontend: ## Build Vue 3 frontend → dist/ (required before go build)
	@printf '\033[32m▶ Building frontend\033[0m\n'
	cd frontend && npm ci && npm run build
	@printf '\033[32m✓ dist/ ready\033[0m\n'

dev-frontend: ## Run Vite dev server on :5173 (proxies /api → :8443)
	cd frontend && npm run dev

# ─── Go backend ───────────────────────────────────────────────────────────────
build: build-frontend ## Build complete binary (frontend + Go)
	@printf '\033[32m▶ Building\033[0m %s %s\n' "$(BINARY)" "$(VERSION)"
	$(GO) build $(BUILD_FLAGS) -o $(BINARY) ./cmd/server/
	@printf '\033[32m✓ Done:\033[0m %s\n' "$(BINARY)"

build-go: ## Build Go binary only (skip frontend; dist/ must exist)
	@printf '\033[32m▶ Building Go only\033[0m %s\n' "$(VERSION)"
	$(GO) build $(BUILD_FLAGS) -o $(BINARY) ./cmd/server/
	@printf '\033[32m✓ Done:\033[0m %s\n' "$(BINARY)"

build-debug: ## Compile with debug symbols (race detector)
	$(GO) build -race -ldflags "-X main.version=$(VERSION)-debug" -o $(BINARY)-debug ./cmd/server/

dev-backend: ## Run Go backend with race detector (needs dist/ from dev-frontend)
	@printf '\033[33m▶ Dev backend\033[0m (race detector on) https://localhost:8443\n'
	$(GO) run -race ./cmd/server/

emulate: ## Run Modbus TCP emulator (serves real DB frames on localhost ports)
	@printf '\033[33m▶ Emulador Modbus\033[0m  DB=correcciones/modbus.db\n'
	$(GO) run ./cmd/emulator/ -db correcciones/modbus.db -cfg config.yaml

run: build ## Build and run the server
	@printf '\033[32m▶ Starting\033[0m https://localhost:8443\n'
	./$(BINARY)

# ─── Code quality ─────────────────────────────────────────────────────────────
fmt: ## Format all Go source files
	$(GO) fmt ./...

vet: ## Run go vet (static analysis)
	$(GO) vet ./...

test: ## Run all tests with race detector
	$(GO) test -race -count=1 -timeout 30s ./...

test-v: ## Run tests with verbose output
	$(GO) test -v -race -count=1 -timeout 30s ./...

check: fmt vet ## Run fmt + vet (pre-commit quality gate)
	@printf '\033[32m✓ All checks passed\033[0m\n'

# ─── Dependencies ─────────────────────────────────────────────────────────────
deps: ## Tidy and verify Go modules
	$(GO) mod tidy
	$(GO) mod verify

deps-upgrade: ## Upgrade all direct dependencies to latest
	$(GO) get -u ./...
	$(GO) mod tidy

# ─── Install ──────────────────────────────────────────────────────────────────
install: build ## Install binary to $GOPATH/bin
	$(GO) install $(BUILD_FLAGS) ./cmd/server/

# ─── Cleanup ──────────────────────────────────────────────────────────────────
clean: ## Remove compiled binaries, dist/, and log files
	$(GO) clean
	rm -f $(BINARY) $(BINARY)-debug
	rm -rf dist/
	@printf '\033[32m✓ Clean\033[0m\n'

clean-logs: ## Remove runtime log files only
	rm -rf logs/
	@printf '\033[32m✓ Logs cleared\033[0m\n'

# ─── Info ─────────────────────────────────────────────────────────────────────
info: ## Print build environment info
	@printf '\n  Binary:   %s\n'  "$(BINARY)"
	@printf   '  Module:   %s\n'  "$(MODULE)"
	@printf   '  Version:  %s\n'  "$(VERSION)"
	@printf   '  Commit:   %s\n'  "$(COMMIT)"
	@printf   '  Date:     %s\n'  "$(DATE)"
	@printf   '  Go:       %s\n'  "$$($(GO) version)"
	@printf   '  OS/Arch:  %s/%s\n\n' "$$($(GO) env GOOS)" "$$($(GO) env GOARCH)"
