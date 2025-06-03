# Makefile for payment-orchestrator

.PHONY: all gen lint test clean

all: gen lint test

# Generate code from protobuf schemas
gen:
	@echo "Generating code from schemas..."
	# Placeholder for actual buf generate command
	# buf generate

# Lint Go files
lint:
	@echo "Linting Go files..."
	# Placeholder for actual lint command
	# golangci-lint run ./...

# Run tests
test:
	@echo "Running tests..."
	# Placeholder for actual test command
	# go test -v ./...

# Clean generated files and build artifacts
clean:
	@echo "Cleaning up..."
	# Placeholder for clean commands
