package server_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

func BenchmarkRepository(b *testing.B) {
	for _, size := range []int{1_000, 10_000, 100_000, 1_000_000} {
		b.Run("users="+strconv.Itoa(size), func(b *testing.B) { benchRepository(b, size) })
	}
}

func benchRepository(b *testing.B, size int) {
	ctx := b.Context()
	repository := server.NewRepository[*core.User](basePath+"/Users", core.Schemas{core.NewSchema(core.SchemaUser).WithName("User").With(userAttributes()...)})
	var seeded *core.User
	for i := range size {
		seeded = seed(ctx, b, repository, "seed"+strconv.Itoa(i))
	}
	query := &protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.MaxCount}
	created := 0
	cases := []struct {
		name string
		run  func() error
	}{
		{"Create", func() error {
			created++
			_, err := repository.Create(ctx, &core.User{UserName: "new" + strconv.Itoa(created)})
			return err
		}},
		{"Get", func() error {
			_, err := repository.Get(ctx, seeded.ID)
			return err
		}},
		{"Replace", func() error {
			seeded.Meta.Version = ""
			_, err := repository.Replace(ctx, seeded)
			return err
		}},
		{"List", func() error {
			_, _, err := repository.List(ctx, query)
			return err
		}},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) { loop(b, tc.run) })
	}
}

func loop(b *testing.B, run func() error) {
	b.ReportAllocs()
	for b.Loop() {
		if err := run(); err != nil {
			b.Fatal(err)
		}
	}
}

func seed(ctx context.Context, b *testing.B, repository server.Repository[*core.User], userName string) *core.User {
	created, err := repository.Create(ctx, &core.User{UserName: userName})
	if err != nil {
		b.Fatal(err)
	}
	return created
}
