.DEFAULT_GOAL := help

.PHONY: lint
lint: ## Lint Go files in both modules
	@golangci-lint run --issues-exit-code 1 ./...
	@cd store/redis && golangci-lint run --issues-exit-code 1 ./...

.PHONY: test
test: ## Run all tests including Valkey integration (requires Docker)
	@go test -v -race ./...
	@go -C store/redis test -v -race -count=1 -timeout=5m ./...

.PHONY: test-unit
test-unit: ## Run unit tests in both modules without Docker
	@go test -short -race ./...
	@go -C store/redis test -short -race ./...

.PHONY: coverage
coverage: ## Run all tests with coverage for both modules (requires Docker)
	@go test -v -race -cover -coverpkg=./... -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out
	@go -C store/redis test -v -race -count=1 -timeout=5m -cover -coverpkg=./... -coverprofile=coverage.out -covermode=atomic ./...
	@go -C store/redis tool cover -func=coverage.out

.PHONY: bench
bench: ## Benchmark memory and GetSet without race instrumentation
	@go test -p=1 -run '^$$' -bench . -benchmem -benchtime=200ms -count=3 -cpu=1,8 ./...

.PHONY: help
help: ## Display this help screen
	@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
