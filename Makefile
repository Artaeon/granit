.PHONY: build run clean test install size completion web-setup web-build web-dev web-serve fetch-strongs

BINARY=granit
MODULE=github.com/artaeon/granit
VERSION=0.1.0
BUILD_DIR=bin
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build: web-build
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/granit/
	@echo "Built $(BUILD_DIR)/$(BINARY) ($$(du -h $(BUILD_DIR)/$(BINARY) | cut -f1))"

# ── web frontend (SvelteKit, embedded into the granit binary) ─────────
web-setup:
	cd web && pnpm install

web-build:
	cd web && pnpm build

web-dev:
	cd web && pnpm dev

# `make web-serve VAULT=~/Documents/Main` boots `granit web --dev` plus Vite.
web-serve:
	@(go run ./cmd/granit web --dev --addr :8787 $(VAULT)) & \
	 (cd web && pnpm dev) ; \
	 wait

run:
	go run ./cmd/granit/ $(ARGS)

clean:
	@rm -rf $(BUILD_DIR)
	@echo "Cleaned."

test:
	go test ./...

install: build
	@cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || \
		cp $(BUILD_DIR)/$(BINARY) ~/go/bin/$(BINARY)
	@echo "Installed $(BINARY) to ~/go/bin/ ($$(du -h $(BUILD_DIR)/$(BINARY) | cut -f1))"

size: build
	@echo "Binary size: $$(du -h $(BUILD_DIR)/$(BINARY) | cut -f1)"
	@echo "To compress further: upx --best $(BUILD_DIR)/$(BINARY)"

# Populate the optional Strong's lexicon + tagged-bible JSONs that
# internal/scripture/bible embeds. The script is interactive-friendly
# (heavily commented, no auto-download of unverified URLs) — see
# scripts/fetch-strongs.sh for details. Rebuild after running so the
# new bytes land in the binary.
fetch-strongs:
	bash scripts/fetch-strongs.sh

# Populate the optional ASV / KJV / BBE translation JSONs that sit
# alongside web.json in internal/scripture/bible. Same playbook as
# fetch-strongs: heavily commented stub, no auto-download. Rebuild
# afterward so go:embed picks up the new bytes.
fetch-translations:
	bash scripts/fetch-bible-translations.sh

scan:
	go run ./cmd/granit/ scan $(VAULT)

daily:
	go run ./cmd/granit/ daily $(VAULT)

completion:
	@echo "# Add one of these to your shell config:"
	@echo "#   granit completion bash >> ~/.bashrc"
	@echo "#   granit completion zsh  >> ~/.zshrc"
	@echo "#   granit completion fish > ~/.config/fish/completions/granit.fish"

# Cross-compilation
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/granit/

build-darwin:
	GOOS=darwin GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/granit/

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/granit/
