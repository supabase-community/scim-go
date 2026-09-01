
test:
	go test -race ./...

run:
	go run ./cmd/server/main.go

format:
	go fmt ./...
	go run golang.org/x/tools/cmd/goimports@latest -w .

lint:
	go vet ./...
	golangci-lint run
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...
