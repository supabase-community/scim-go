.DEFAULT_GOAL := build

BINARY := scim-server
CMD := ./cmd/server
COVERAGE := coverage.out
COVER_MIN := 80
COVER_EXCLUDE := /pkg/scimtest/|/internal/|/cmd/

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
	CGO_ENABLED=1 go test -race ./...

.PHONY: cover
cover:
	go test -race -coverprofile=$(COVERAGE) ./...
	grep -Ev '$(COVER_EXCLUDE)' $(COVERAGE) > $(COVERAGE).tmp && mv $(COVERAGE).tmp $(COVERAGE)
	go tool cover -func=$(COVERAGE)

.PHONY: cover-check
cover-check: cover
	@go tool cover -func=$(COVERAGE) | awk -v min=$(COVER_MIN) '/^total:/ { \
	  gsub(/%/, "", $$3); \
	  if ($$3 + 0 < min) { printf "coverage %.1f%% is below minimum %d%%\n", $$3, min; exit 1 } \
	}'

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

.PHONY: ci
ci: tidy fmt lint vulncheck cover-check

.PHONY: help
help: ## List available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "%-10s %s\n", $$1, $$2}'
