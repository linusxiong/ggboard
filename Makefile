# GG Bond (ggboard) build targets. Run `make help` for the list.
GO        ?= go
PNPM      ?= pnpm
BIN       ?= bin/ggboard
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
STATIC    := internal/static/dist

.PHONY: help build run test lint web e2e tidy clean

help: ## Show targets
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-8s %s\n", $$1, $$2}'

build: ## Build the static binary into bin/ggboard (embeds internal/static/dist)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/ggboard

run: ## Run the server from source (data in ./data)
	$(GO) run ./cmd/ggboard serve

test: ## Run Go tests with the race detector
	$(GO) test ./... -race -count=1

lint: ## golangci-lint when installed, else go vet + gofmt check
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found; running go vet and gofmt"; \
		$(GO) vet ./... && out="$$(gofmt -l cmd internal)" && test -z "$$out" || { echo "$$out"; exit 1; }; \
	fi

web: ## Build the frontend and copy web/dist into internal/static/dist
	$(PNPM) -C web install
	$(PNPM) -C web build
	find $(STATIC) -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
	cp -R web/dist/. $(STATIC)/

e2e: ## Build, start the server on a random port and smoke-test it
	./scripts/e2e.sh

tidy: ## go mod tidy
	$(GO) mod tidy

clean: ## Remove build output and the copied frontend
	rm -rf bin
	find $(STATIC) -mindepth 1 ! -name .gitkeep -exec rm -rf {} +
