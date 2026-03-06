.PHONY: build run clean test install

BINARY=granit
MODULE=github.com/artaeon/granit
VERSION=0.1.0
BUILD_DIR=bin

build:
	@echo "Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) ./cmd/granit/

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
	@echo "Installed $(BINARY)"

scan:
	go run ./cmd/granit/ scan $(VAULT)

daily:
	go run ./cmd/granit/ daily $(VAULT)

# Cross-compilation
build-all: build-linux build-darwin build-windows

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-linux-amd64 ./cmd/granit/

build-darwin:
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-darwin-arm64 ./cmd/granit/

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe ./cmd/granit/
