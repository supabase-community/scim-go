.DEFAULT_GOAL := build

COVERAGE := coverage.out
COVER_MIN := 95
COVER_EXCLUDE := /pkg/scimtest/|/internal/|/cmd/server/
BENCHCOUNT ?= 1
FUZZTIME ?= 30s

.PHONY: clean
clean:
	rm -rf $(COVERAGE)

.PHONY: build
build:
	go build -o bin/scim-server ./cmd/server

.PHONY: run
run:
	go run ./cmd/server

.PHONY: smoke
smoke: build
	scripts/smoke-test.sh

.PHONY: test
test:
	CGO_ENABLED=1 go test -race ./...

.PHONY: bench
bench:
	go test -run '^$$' -bench=. -benchmem -count $(BENCHCOUNT) ./...

.PHONY: fuzz
fuzz:
	grep -rHo --include='*_test.go' '^func Fuzz[A-Za-z0-9_]*' . \
	  | sed -E 's#^(\./)?(.*)/[^/]*:func (Fuzz.*)#go test -run "^$$" -fuzz "^\3$$" -fuzztime $(FUZZTIME) ./\2#' \
	  | sh -e

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
	golangci-lint fmt

.PHONY: lint
lint:
	golangci-lint run

.PHONY: vulncheck
vulncheck:
	go tool govulncheck ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: ci
ci: tidy fmt lint vulncheck cover-check smoke
