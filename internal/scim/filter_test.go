package scim

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func bjensen() *core.User {
	return &core.User{
		UserName: "bjensen",
		Name:     core.Name{FamilyName: "Jensen", GivenName: "Barbara"},
		Active:   new(true),
		Emails: []core.Email{
			{Value: "bjensen@work.example", Type: "work", Primary: new(true)},
			{Value: "barbara@home.example", Type: "home"},
		},
	}
}

func jsmith() *core.User {
	return &core.User{
		UserName: "jsmith",
		Name:     core.Name{FamilyName: "Smith"},
		Active:   new(false),
	}
}

func TestMatchesUser(t *testing.T) {
	tt := []struct {
		name   string
		filter string
		user   *core.User
		want   bool
	}{
		{"eq match", `userName eq "bjensen"`, bjensen(), true},
		{"eq is case-insensitive on value", `userName eq "BJENSEN"`, bjensen(), true},
		{"eq is case-insensitive on attribute", `USERNAME eq "bjensen"`, bjensen(), true},
		{"eq no match", `userName eq "bjensen"`, jsmith(), false},
		{"ne match", `userName ne "bjensen"`, jsmith(), true},
		{"ne does not over-match a multi-valued attribute that contains the value", `emails.type ne "work"`, bjensen(), false},
		{"ne on an absent attribute is true", `nickName ne "x"`, bjensen(), true},
		{"co match", `name.familyName co "ens"`, bjensen(), true},
		{"sw match", `userName sw "bj"`, bjensen(), true},
		{"ew match", `userName ew "sen"`, bjensen(), true},
		{"active eq true", `active eq true`, bjensen(), true},
		{"active eq true on inactive user", `active eq true`, jsmith(), false},
		{"presence of set attribute", `userName pr`, bjensen(), true},
		{"presence of unset attribute", `nickName pr`, bjensen(), false},
		{"gt on strings", `userName gt "a"`, bjensen(), true},
		{"lt on strings", `userName lt "a"`, bjensen(), false},
		{"and both true", `userName eq "bjensen" and active eq true`, bjensen(), true},
		{"and one false", `userName eq "bjensen" and active eq false`, bjensen(), false},
		{"or one true", `userName eq "nope" or active eq true`, bjensen(), true},
		{"not inverts", `not (userName eq "bjensen")`, bjensen(), false},
		{"multi-valued any element matches", `emails.type eq "home"`, bjensen(), true},
		{"value path type", `emails[type eq "work"]`, bjensen(), true},
		{"value path no match", `emails[type eq "other"]`, bjensen(), false},
		{"value path with sub-attribute and and/or", `emails[type eq "work" and primary eq true]`, bjensen(), true},
		{"value path with sub-attribute present", `emails[type eq "work"].value`, bjensen(), true},
		{"schema-prefixed core attribute", `urn:ietf:params:scim:schemas:core:2.0:User:userName eq "bjensen"`, bjensen(), true},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			pred, err := compileFilter(tc.filter)
			require.NoError(t, err)

			got, err := matches(tc.user, pred)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFilterUsers(t *testing.T) {
	users := []*core.User{bjensen(), jsmith()}

	t.Run("narrows to matching users", func(t *testing.T) {
		matched, err := filterResources(users, `active eq true`)

		require.NoError(t, err)
		require.Len(t, matched, 1)
		assert.Equal(t, "bjensen", matched[0].UserName)
	})

	t.Run("invalid filter is an invalidFilter error", func(t *testing.T) {
		_, err := filterResources(users, `userName eq`)

		require.ErrorIs(t, err, protocol.ErrInvalidFilter(""))
	})

	t.Run("deeply nested filter is rejected fast, not parsed", func(t *testing.T) {
		bomb := strings.Repeat("(", 5000) + `userName eq "x"` + strings.Repeat(")", 5000)

		_, err := filterResources(users, bomb)

		require.ErrorIs(t, err, protocol.ErrInvalidFilter(""))
	})

	t.Run("over-long filter is rejected", func(t *testing.T) {
		long := `userName eq "` + strings.Repeat("a", maxFilterLength) + `"`

		_, err := filterResources(users, long)

		require.ErrorIs(t, err, protocol.ErrInvalidFilter(""))
	})
}
