# scim-go

Building blocks for SCIM 2.0 servers in Go. Implements IETF [RFC-7643][1] (schema) and [RFC-7644][2] (protocol).

> [!WARNING]
> Pre-1.0. The public API may still change between minor versions.

## Quick start

Run the reference server, `scimd`, with an in-memory store:

```
SCIM_BEARER_TOKEN=secret go run github.com/supabase-community/scim-go/cmd/scimd@latest
```

```
curl -X POST http://localhost:8080/scim/v2/Users \
  -H "Authorization: Bearer secret" \
  -H "Content-Type: application/scim+json" \
  -d '{"userName":"bjensen"}'
```

Check any SCIM server against the RFCs with `scimt`:

```
go run github.com/supabase-community/scim-go/cmd/scimt@latest -url https://host/scim/v2 -token secret -strict
```

`scimd` is a demo of the library. It is not meant for production. Prebuilt binaries for Linux, macOS, and Windows are on the [releases page](https://github.com/supabase-community/scim-go/releases).

| Variable            | Purpose                                          | Default    |
| ------------------- | ------------------------------------------------ | ---------- |
| `PORT`              | Listen port                                      | 8080       |
| `SCIM_BASE_URL`     | Public base URL that `meta.location` starts with | `/scim/v2` |
| `SCIM_BEARER_TOKEN` | Bearer token clients must send (required)        | none       |

## Use as a library

```
go get github.com/supabase-community/scim-go
```

```go
config := core.NewServiceProviderConfig().Filtering(protocol.DefaultLimits.MaxCount).Patching()

handler := server.New("/scim/v2", config,
	server.WithResource(server.NewResource[*core.User](
		"User", "/Users", core.SchemaUser, core.UserAttributes()...,
	)),
	server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(),
		server.RequireBearerToken(validateToken),
	),
)

http.ListenAndServe(":8080", handler)
```

Replace the in-memory store with `Resource.WithRepository`. Wrap business logic with `Resource.WithService`. See `ExampleNew` in `pkg/server` for a compiled version and `cmd/scimd/main.go` for a complete server.

## Packages

- `pkg/core`: SCIM 2.0 core schema types (RFC 7643): attributes, schemas, resource types.
- `pkg/filter`: SCIM 2.0 filter grammar and parser (RFC 7644 3.4.2.2).
- `pkg/patch`: SCIM 2.0 patch support (RFC 7644 3.5.2).
- `pkg/protocol`: SCIM 2.0 protocol types (RFC 7644): list responses, filters, and helpers for writing SCIM shaped HTTP responses.
- `pkg/scimerrors`: SCIM 2.0 errors (RFC 7644): errors SCIM shaped HTTP responses.
- `pkg/scimtest`: checks a SCIM implementation against the JSON examples in RFC 7643.
- `pkg/server`: SCIM 2.0 server components for building a customized SCIM Server. Experimental API.

## RFC coverage

| Feature                                       | RFC              | Status      |
|-----------------------------------------------|------------------|-------------|
| User, Group, Enterprise User                  | 7643 4           | Supported   |
| ServiceProviderConfig, ResourceTypes, Schemas | 7643 5-7, 7644 4 | Supported   |
| Create, read, replace, delete                 | 7644 3.3-3.6     | Supported   |
| Filtering, sorting, pagination, attributes    | 7644 3.4.2       | Supported   |
| Query with POST `/.search`                    | 7644 3.4.3       | Supported   |
| PATCH                                         | 7644 3.5.2       | Supported   |
| ETags and versioning                          | 7644 3.14        | Supported   |
| Bulk                                          | 7644 3.7         | Returns 501 |
| `/Me`                                         | 7644 3.11        | Returns 501 |

[1]: https://datatracker.ietf.org/doc/html/rfc7643
[2]: https://datatracker.ietf.org/doc/html/rfc7644
