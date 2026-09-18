package server

import "github.com/supabase-community/scim-go/pkg/core"

type Accessor[T Entity] func(item T) any

type Accessors[T Entity] map[*core.Attribute]Accessor[T]
