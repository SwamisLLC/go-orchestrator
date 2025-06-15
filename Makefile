# Makefile

# Variables
GO := go
BUF := buf
GOLANGCI_LINT := golangci-lint

# Default target
all: gen lint test

# Targets
gen:
	@echo "Generating Go code from Protobuf..."
	@$(BUF) generate

lint:
	@echo "Linting Go code..."
	@$(GOLANGCI_LINT) run ./...

test:
	@echo "Running Go tests with race detection and coverage..."
	@$(GO) test -race -coverprofile=coverage.out -covermode=atomic ./...

ci: gen lint test
	@echo "CI pipeline completed successfully."

clean:
	@echo "Cleaning up generated files and build artifacts..."
	@$(GO) clean -testcache
	@rm -f coverage.out main
	# Consider if pkg/gen/* should be cleaned, often it's kept
	# @rm -rf pkg/gen/*

.PHONY: all gen lint test ci clean
