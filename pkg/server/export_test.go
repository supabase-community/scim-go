package server

import (
	"github.com/supabase-community/scim-go/internal/memory"
	"github.com/supabase-community/scim-go/pkg/core"
)

var NewDatabase = memory.New

func NewRepositoryIn[T core.Resource](db *memory.Memory, endpoint string, schemas core.Schemas) Repository[T] {
	return NewRepository[T](db.Store(endpoint, schemas), schemas)
}

func WithDatabase(db *memory.Memory) Option[*Server] {
	return func(s *Server) { s.db = db }
}

func NewMemoryRepository[T core.Resource](endpoint string, schemas core.Schemas) Repository[T] {
	return NewRepository[T](memory.New().Store(endpoint, schemas), schemas)
}
