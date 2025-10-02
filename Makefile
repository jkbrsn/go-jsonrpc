.PHONY: test bench build fmt lint

# Run all tests
test:
	go test ./... -count=1

# Run benchmarks
bench:
	go test -bench=. -benchmem -run=^$$ ./...

# Build verification
build:
	go build ./...

# Format code
fmt:
	go fmt ./...

# Run linter (requires golangci-lint)
lint:
	golangci-lint run

# Run tests with coverage
cover:
	go test ./... -cover
