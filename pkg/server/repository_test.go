package server_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

// RFC 7643 Section 7: a "server" unique value SHOULD be unique within the context of the current SCIM endpoint.
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

func TestRepositoryListBounds(t *testing.T) {
	cases := []struct {
		name  string
		query *protocol.SearchRequest
		want  int
	}{
		{"negative count", &protocol.SearchRequest{StartIndex: 1, Count: -1}, 0},
		{"count past the end", &protocol.SearchRequest{StartIndex: 2, Count: math.MaxInt}, 1},
		{"start past the end", &protocol.SearchRequest{StartIndex: 10, Count: 5}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository := server.NewRepository[*core.User](basePath+"/Users", userSchemas())
			for _, name := range []string{"a", "b"} {
				_, err := repository.Create(context.Background(), &core.User{UserName: name})
				require.NoError(t, err)
			}

			items, total, err := repository.List(context.Background(), tc.query)

			require.NoError(t, err)
			assert.Equal(t, 2, total)
			assert.Len(t, items, tc.want)
		})
	}
}
