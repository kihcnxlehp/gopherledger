.PHONY: build run test test-cover lint fmt clean

# Build the binary
build:
	go build -o bin/gopherledger ./cmd/server

# Run the server
run:
	go run ./cmd/server

# Run all tests with race detector
test:
	go test -race -cover ./...

# Run tests with HTML coverage report
test-cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter (requires golangci-lint)
lint:
	golangci-lint run ./...

# Format code
fmt:
	gofmt -s -w .

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html stats.txt