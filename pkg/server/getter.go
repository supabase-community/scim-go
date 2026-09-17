package server

type Getter[T Entity] func(item T) any

// RFC 7644 3.4.2.2 - a []any Getter result represents a multi-valued attribute.
type Getters[T Entity] map[string]Getter[T]
