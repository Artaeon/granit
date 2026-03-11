.PHONY: build run clean test install size completion

BINARY=granit
MODULE=github.com/artaeon/granit
VERSION=0.1.0
BUILD_DIR=bin
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/granit/
	@echo "Built $(BUILD_DIR)/$(BINARY) ($$(du -h $(BUILD_DIR)/$(BINARY) | cut -f1))"

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
