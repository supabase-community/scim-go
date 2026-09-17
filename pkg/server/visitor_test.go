package server_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

func newTestSchema() *core.Schema {
	return (&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
		core.NewAttribute("userName", core.TypeString),
		core.NewAttribute("active", core.TypeBoolean),
		core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
		),
		core.NewAttribute("password", core.TypeString),
	)
}

func newTestGetters() server.Getters[*core.User] {
	return server.Getters[*core.User]{
		"username": func(u *core.User) any { return u.UserName },
		"active": func(u *core.User) any {
			if u.Active == nil {
				return nil
			}
			return *u.Active
		},
		"emails.value": func(u *core.User) any {
			values := make([]any, len(u.Emails))
			for i, e := range u.Emails {
				values[i] = e.Value
			}
			return values
		},
		"meta.lastmodified": func(u *core.User) any { return u.Meta.LastModified },
	}
}

func TestVisitorFilter(t *testing.T) {
	schema := newTestSchema()
	schemas := []*core.Schema{schema}
	getters := newTestGetters()

	t.Run("eq matches userName case-insensitively by default", func(t *testing.T) {
		predicate, err := protocol.Filter(schemas, `userName eq "ALICE"`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, predicate(&core.User{UserName: "alice"}))
	})

	t.Run("eq respects caseExact", func(t *testing.T) {
		exactSchema := (&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("userName", core.TypeString).AsCaseExact(),
		)
		exactGetters := server.Getters[*core.User]{
			"username": func(u *core.User) any { return u.UserName },
		}
		predicate, err := protocol.Filter([]*core.Schema{exactSchema}, `userName eq "Bob"`, server.NewVisitor(exactGetters))
		require.NoError(t, err)
		assert.False(t, predicate(&core.User{UserName: "bob"}))
		assert.True(t, predicate(&core.User{UserName: "Bob"}))
	})

	t.Run("comparisons against an absent value are false, including ne", func(t *testing.T) {
		user := &core.User{UserName: "alice"}

		eq, err := protocol.Filter(schemas, `active eq true`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.False(t, eq(user))

		ne, err := protocol.Filter(schemas, `active ne true`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.False(t, ne(user))
	})

	t.Run("boolean eq matches a set value", func(t *testing.T) {
		predicate, err := protocol.Filter(schemas, `active eq true`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, predicate(&core.User{UserName: "alice", Active: new(true)}))
		assert.False(t, predicate(&core.User{UserName: "alice", Active: new(false)}))
	})

	t.Run("multi-valued co matches if any element matches", func(t *testing.T) {
		predicate, err := protocol.Filter(schemas, `emails.value co "@example.com"`, server.NewVisitor(getters))
		require.NoError(t, err)
		user := &core.User{Emails: []core.Email{
			{Value: "alice@other.example"},
			{Value: "alice@example.com"},
		}}
		assert.True(t, predicate(user))
		assert.False(t, predicate(&core.User{Emails: []core.Email{{Value: "alice@other.example"}}}))
	})

	t.Run("pr is true only for a non-empty value", func(t *testing.T) {
		predicate, err := protocol.Filter(schemas, `userName pr`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, predicate(&core.User{UserName: "alice"}))
		assert.False(t, predicate(&core.User{UserName: ""}))
	})

	t.Run("pr is true for a defined boolean, even false", func(t *testing.T) {
		predicate, err := protocol.Filter(schemas, `active pr`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, predicate(&core.User{Active: new(false)}))
		assert.False(t, predicate(&core.User{}))
	})

	t.Run("string ordering operators", func(t *testing.T) {
		ge, err := protocol.Filter(schemas, `userName ge "alice"`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, ge(&core.User{UserName: "bob"}))
		assert.False(t, ge(&core.User{UserName: "aardvark"}))

		le, err := protocol.Filter(schemas, `userName le "bob"`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, le(&core.User{UserName: "alice"}))
		assert.False(t, le(&core.User{UserName: "carol"}))
	})

	t.Run("and, or, not compose predicates", func(t *testing.T) {
		user := &core.User{UserName: "alice", Active: new(true)}

		and, err := protocol.Filter(schemas, `userName eq "alice" and active eq true`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, and(user))
		assert.False(t, and(&core.User{UserName: "alice", Active: new(false)}))

		or, err := protocol.Filter(schemas, `userName eq "bob" or userName eq "alice"`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, or(user))

		not, err := protocol.Filter(schemas, `not (userName eq "bob")`, server.NewVisitor(getters))
		require.NoError(t, err)
		assert.True(t, not(user))
	})

	t.Run("a schema attribute with no registered getter is rejected", func(t *testing.T) {
		_, err := protocol.Filter(schemas, `password eq "secret"`, server.NewVisitor(getters))
		require.Error(t, err)
		assert.ErrorIs(t, err, scimerrors.ErrInvalidFilter(""))
	})

	t.Run("dateTime comparisons use calendar time, not string ordering", func(t *testing.T) {
		predicate, err := protocol.Filter(schemas, `meta.lastModified gt "2024-01-01T00:00:00Z"`, server.NewVisitor(getters))
		require.NoError(t, err)
		user := &core.User{Meta: core.Meta{LastModified: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)}}
		assert.True(t, predicate(user))

		earlier := &core.User{Meta: core.Meta{LastModified: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)}}
		assert.False(t, predicate(earlier))
	})
}
