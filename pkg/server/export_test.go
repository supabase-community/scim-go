package server

import "github.com/supabase-community/scim-go/pkg/core"

var NewDatabase = newDatabase

func NewRepositoryIn[T core.Resource](db *database, endpoint string, schemas core.Schemas) Repository[T] {
	return newRepository[T](db, endpoint, schemas)
}

func WithDatabase(db *database) Option[*Server] {
	return func(s *Server) { s.db = db }
}
