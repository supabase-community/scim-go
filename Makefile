.DEFAULT_GOAL := build

BINARY := scim-server
CMD := ./cmd/server
COVERAGE := coverage.out

.PHONY: clean
clean:
	rm -rf bin $(COVERAGE)

.PHONY: build
build:
	go build -o bin/$(BINARY) $(CMD)

.PHONY: run
run:
	go run $(CMD)

.PHONY: test
test:
	go test -race ./...

.PHONY: cover
cover:
	go test -race -coverprofile=$(COVERAGE) ./...
	go tool cover -func=$(COVERAGE)

.PHONY: fmt
fmt:
	gofmt -l -w .
	go tool goimports -w .

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint: vet
	golangci-lint run

.PHONY: vulncheck
vulncheck:
	go tool govulncheck ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: check
check: generate tidy fmt lint vulncheck test

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-10s %s\n", $$1, $$2}'
