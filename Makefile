# AIChain Makefile

.PHONY: build install clean test run dev

# Build the binary
build:
	go build -o bin/aichain ./cmd/aichain

# Build for multiple platforms
build-all:
	GOOS=darwin GOARCH=amd64 go build -o bin/aichain-darwin-amd64 ./cmd/aichain
	GOOS=darwin GOARCH=arm64 go build -o bin/aichain-darwin-arm64 ./cmd/aichain
	GOOS=linux GOARCH=amd64 go build -o bin/aichain-linux-amd64 ./cmd/aichain
	GOOS=windows GOARCH=amd64 go build -o bin/aichain-windows-amd64.exe ./cmd/aichain

# Install locally for development
install: build
	cp bin/aichain /usr/local/bin/aichain
	chmod +x /usr/local/bin/aichain

# Clean build artifacts
clean:
	rm -rf bin/

# Run tests
test:
	go test ./...

# Run the server for development
run: build
	./bin/aichain --server --port 8747

# Development mode with auto-restart (requires air: go install github.com/cosmtrek/air@latest)
dev:
	air -c .air.toml

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Show help
help:
	@echo "Available commands:"
	@echo "  build      - Build the aichain binary"
	@echo "  build-all  - Build for multiple platforms"
	@echo "  install    - Install binary to /usr/local/bin"
	@echo "  clean      - Clean build artifacts"
	@echo "  test       - Run tests"
	@echo "  run        - Run the server for development"
	@echo "  dev        - Run with auto-restart (requires air)"
	@echo "  deps       - Download and tidy dependencies"
	@echo "  fmt        - Format code"
	@echo "  lint       - Lint code"