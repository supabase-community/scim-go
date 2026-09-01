module mokhan.ca/go/scim

go 1.27.0

require (
	github.com/gofrs/uuid v4.4.0+incompatible
	github.com/stretchr/testify v1.12.1
)

require (
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.39.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/telemetry v0.0.0-20260811182544-a038080d80e5 // indirect
	golang.org/x/tools v0.49.0 // indirect
	golang.org/x/vuln v1.7.0 // indirect
)

tool (
	golang.org/x/tools/cmd/goimports
	golang.org/x/tools/cmd/goyacc
	golang.org/x/vuln/cmd/govulncheck
)
