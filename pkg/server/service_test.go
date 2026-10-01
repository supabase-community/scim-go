package server_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

// RFC 7643 Section 2.2: a service built with NewService enforces attribute characteristics.
func TestNewServiceEnforcesCharacteristics(t *testing.T) {
	repo := server.NewRepository[*core.Group](basePath+"/Groups", groupSchemas())
	service := server.NewService(repo, groupSchemas(), server.Limits{})

	_, err := service.Create(context.Background(), core.Object{"schemas": []any{string(core.SchemaGroup)}})

	assert.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
}
