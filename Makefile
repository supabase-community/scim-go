.DEFAULT_GOAL := build

BINARY := scim-server
CMD := ./cmd/server
COVERAGE := coverage.out

.PHONY: build
build: ## Build the reference server binary
	go build -o bin/$(BINARY) $(CMD)

.PHONY: run
run: ## Run the reference server
	go run $(CMD)

.PHONY: test
test: ## Run tests with the race detector
	go test -race ./...

.PHONY: cover
cover: ## Run tests and report coverage
	go test -race -coverprofile=$(COVERAGE) ./...
	go tool cover -func=$(COVERAGE)

.PHONY: fmt
fmt: ## Format source files
	gofmt -l -w .
	go tool goimports -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: vet ## Run golangci-lint
	golangci-lint run

.PHONY: vulncheck
vulncheck: ## Check dependencies for known vulnerabilities
	go tool govulncheck ./...

.PHONY: tidy
tidy: ## Tidy go.mod and go.sum
	go mod tidy

.PHONY: check
check: tidy fmt lint vulncheck test ## Run the full suite of checks

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin $(COVERAGE)

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-10s %s\n", $$1, $$2}'
