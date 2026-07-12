.DEFAULT_GOAL := help

# --- Config (override on the command line, e.g. `make generate DSN=...`) -----

DSN         ?= postgres://postgres:example@localhost:5432/dbname
FILE        ?= schema.erd
ADDR        ?= 127.0.0.1:0
BIN         ?= bin/erdlens
GOPROXY_URL ?= https://proxy.golang.org,direct

GO_LDFLAGS  := -s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# --- Meta --------------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@awk 'BEGIN{FS=":.*##"; printf "\nUsage: make \033[36m<target>\033[0m [VAR=value]\n\nTargets:\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } \
		/^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
	@echo
	@echo "Common variables:"
	@echo "  DSN=$(DSN)"
	@echo "  FILE=$(FILE)"
	@echo "  ADDR=$(ADDR)"
	@echo

##@ Setup

.PHONY: deps
deps: deps-go deps-web ## Install all dependencies (Go + npm)

.PHONY: deps-go
deps-go: ## Fetch Go modules (via public proxy)
	GOPROXY=$(GOPROXY_URL) go mod tidy

.PHONY: deps-web
deps-web: ## Install frontend npm dependencies
	cd web && npm install

##@ Build

.PHONY: build
build: web build-go ## Build frontend + Go binary (single artifact in $(BIN))

.PHONY: build-go
build-go: ## Build the Go binary (assumes web/dist is already built)
	@mkdir -p bin
	go build -ldflags="$(GO_LDFLAGS)" -o $(BIN) ./cmd/erdlens
	@echo "→ $(BIN) ($$(du -h $(BIN) | cut -f1))"

.PHONY: web
web: ## Build the Svelte frontend into web/dist/
	cd web && npm run build

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin web/dist/assets web/dist/index-*.html
	@echo "→ cleaned"

##@ Run

.PHONY: generate
generate: build-go ## Introspect $(DSN) and write $(FILE)
	./$(BIN) generate --dsn '$(DSN)' -o $(FILE)
	@echo "→ wrote $(FILE)"

.PHONY: view
view: build-go ## Open the viewer on $(FILE)
	./$(BIN) view $(FILE) --addr $(ADDR)

.PHONY: view-headless
view-headless: build-go ## Serve the viewer without opening a browser
	./$(BIN) view $(FILE) --addr $(ADDR) --no-browser

.PHONY: run
run: build-go ## Introspect $(DSN), save $(FILE), and open the viewer
	./$(BIN) view --dsn '$(DSN)' -o $(FILE) --addr $(ADDR)

##@ Dev loop

.PHONY: dev-api
dev-api: build-go ## Run the Go server on port 8787 (pair with `make dev-web`)
	./$(BIN) view $(FILE) --addr 127.0.0.1:8787 --no-browser

.PHONY: dev-web
dev-web: ## Run Vite dev server (proxies /api to :8787). Pair with `make dev-api`
	cd web && npm run dev

##@ Test & check

.PHONY: test
test: ## Run all Go tests
	go test ./...

.PHONY: test-roundtrip
test-roundtrip: ## Round-trip parse+write $(FILE) and assert byte-identical
	ERDLENS_ROUNDTRIP_FILE=$(PWD)/$(FILE) go test -v -count=1 -run TestRealWorldRoundTrip ./internal/erdfile/

.PHONY: vet
vet: ## go vet
	go vet ./...

.PHONY: check
check: vet test ## Vet + test

.PHONY: check-offline
check-offline: web ## Verify built dist/ contains no external URLs
	@if grep -rE 'https?://(?!localhost|127\.0\.0\.1)' web/dist/ >/dev/null 2>&1; then \
		echo "❌ external URLs found in web/dist/:"; \
		grep -rnE 'https?://(?!localhost|127\.0\.0\.1)' web/dist/ | head -20; \
		exit 1; \
	else \
		echo "✅ no external URLs in web/dist/"; \
	fi
