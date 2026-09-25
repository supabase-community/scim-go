package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

// RFC 7643 Section 7: "server" uniqueness holds for every attribute type.
func TestRepositoryUniqueness(t *testing.T) {
	noon := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name          string
		first, second *widget
	}{
		{"one instant in two time zones", &widget{When: noon}, &widget{When: noon.In(time.FixedZone("EST", -5*60*60))}},
		{"an element of a complex multi-valued attribute", &widget{Parts: []part{{Serial: "a"}, {Serial: "b"}}}, &widget{Parts: []part{{Serial: "b"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository := server.NewRepository[*widget]("/Widgets", core.Schemas{core.NewSchema(widgetSchema).WithName("Widget").With(
				core.NewAttribute("when", core.TypeDateTime).UniqueOn(core.UniquenessServer),
				core.NewAttribute("parts", core.TypeComplex).AsMultiValued().UniqueOn(core.UniquenessServer).With(
					core.NewAttribute("serial", core.TypeString),
				),
			)})
			_, err := repository.Create(context.Background(), tc.first)
			require.NoError(t, err)

			_, err = repository.Create(context.Background(), tc.second)

			assert.ErrorIs(t, err, scimerrors.ErrUniqueness(""))
		})
	}
}
