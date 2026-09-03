# scim-go

This package implements the SCIM IETF [RFC-7643][1] and [RFC-7644][2] specifications.

> [!WARNING] Pre-1.0. The public API may still change between minor versions.

## Install

```
go get github.com/supabase-community/scim-go
```

## Packages

- `pkg/core`: SCIM 2.0 core schema types (RFC 7643): attributes, schemas, resource types.
- `pkg/protocol`: SCIM 2.0 protocol types (RFC 7644): errors, list responses, and helpers for writing SCIM shaped HTTP responses.
- `pkg/scimtest`: checks a SCIM implementation against the JSON examples in RFC 7643.

[1]: https://datatracker.ietf.org/doc/html/rfc7643
[2]: https://datatracker.ietf.org/doc/html/rfc7644
