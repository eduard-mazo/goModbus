# ─────────────────────────────────────────────────────────────────────────────
# ROC Modbus Expert | EPM — Makefile
# ─────────────────────────────────────────────────────────────────────────────

BINARY  := modbus_client
MODULE  := $(shell go list -m 2>/dev/null)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
PORT    := 8081

# Inject build metadata via ldflags
LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.buildDate=$(DATE)

# Build flags
BUILD_FLAGS := -trimpath -ldflags "$(LDFLAGS)"

# Go tool
GO := go

.DEFAULT_GOAL := help

.PHONY: all build run dev test vet fmt check clean deps install help

# ─── Help ─────────────────────────────────────────────────────────────────────
help: ## Show this help message
	@printf '\n\033[1;32m ROC Modbus Expert — EPM\033[0m  \033[0;90m$(VERSION) ($(COMMIT))\033[0m\n\n'
	@printf ' \033[1mUsage:\033[0m  make \033[36m<target>\033[0m\n\n'
	@printf ' \033[1mTargets:\033[0m\n'
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "   \033[36m%-12s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf '\n'

# ─── Build ────────────────────────────────────────────────────────────────────
all: check build ## Run checks then build

build: ## Compile binary (output: ./modbus_client)
	@printf '\033[32m▶ Building\033[0m %s %s\n' "$(BINARY)" "$(VERSION)"
	$(GO) build $(BUILD_FLAGS) -o $(BINARY) .
	@printf '\033[32m✓ Done:\033[0m %s  (%s)\n' "$(BINARY)" "$$(du -sh $(BINARY) | cut -f1)"

build-debug: ## Compile with debug symbols (no -s -w, enables pprof)
	$(GO) build -race -ldflags "-X main.version=$(VERSION)-debug" -o $(BINARY)-debug .

# ─── Run ──────────────────────────────────────────────────────────────────────
run: build ## Build and run the server (http://localhost:$(PORT))
	@printf '\033[32m▶ Starting\033[0m http://localhost:$(PORT)\n'
	./$(BINARY)

dev: ## Run with race detector and live-reload friendly flags
	@printf '\033[33m▶ Dev mode\033[0m (race detector on) http://localhost:$(PORT)\n'
	$(GO) run -race .

# ─── Code quality ─────────────────────────────────────────────────────────────
fmt: ## Format all Go source files
	@printf '\033[32m▶ Formatting\033[0m\n'
	$(GO) fmt ./...
	@printf '\033[32m✓ Done\033[0m\n'

vet: ## Run go vet (static analysis)
	@printf '\033[32m▶ Vet\033[0m\n'
	$(GO) vet ./...
	@printf '\033[32m✓ Vet passed\033[0m\n'

test: ## Run all tests with race detector
	@printf '\033[32m▶ Testing\033[0m\n'
	$(GO) test -race -count=1 -timeout 30s ./...

test-v: ## Run tests with verbose output
	$(GO) test -v -race -count=1 -timeout 30s ./...

check: fmt vet ## Run fmt + vet (pre-commit quality gate)
	@printf '\033[32m✓ All checks passed\033[0m\n'

# ─── Dependencies ─────────────────────────────────────────────────────────────
deps: ## Tidy and verify Go modules
	@printf '\033[32m▶ Tidying modules\033[0m\n'
	$(GO) mod tidy
	$(GO) mod verify
	@printf '\033[32m✓ Modules OK\033[0m\n'

deps-upgrade: ## Upgrade all direct dependencies to latest
	$(GO) get -u ./...
	$(GO) mod tidy

# ─── Install / Uninstall ──────────────────────────────────────────────────────
install: build ## Install binary to $GOPATH/bin
	@printf '\033[32m▶ Installing\033[0m to $(GOPATH)/bin/$(BINARY)\n'
	$(GO) install $(BUILD_FLAGS) .

# ─── Cleanup ──────────────────────────────────────────────────────────────────
clean: ## Remove compiled binaries and log files
	@printf '\033[32m▶ Cleaning\033[0m\n'
	$(GO) clean
	@rm -f $(BINARY) $(BINARY)-debug
	@printf '\033[32m✓ Clean\033[0m\n'

clean-logs: ## Remove runtime log files only (keeps binary)
	@rm -rf logs/
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
