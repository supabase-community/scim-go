package server

import "github.com/supabase-community/scim-go/pkg/core"

type Getter[T Entity] func(item T) any

type Getters[T Entity] map[*core.Attribute]Getter[T]
