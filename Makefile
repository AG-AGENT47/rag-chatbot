.PHONY: run build test deps lint clean

# Run the server from the repo root (frontend/index.html served relative to here)
run:
	go run ./cmd/server

# Build a production binary
build:
	go build -o bin/server ./cmd/server

# Download and tidy dependencies
deps:
	go mod tidy
	go mod download

# Run all tests
test:
	go test ./...

# Lint with golangci-lint (install: brew install golangci-lint)
lint:
	golangci-lint run ./...

# Remove build artifacts
clean:
	rm -rf bin/
