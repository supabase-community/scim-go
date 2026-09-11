package filter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterConformance(t *testing.T) {
	// RFC 7644 3.4.2.2 filter examples, verified against filter.Parse.
	valid := []string{
		`userName eq "bjensen"`,
		`userName sw "b"`,
		`name.familyName co "O'Malley"`,
		`title pr`,
		`meta.lastModified gt "2011-05-13T04:42:34Z"`,
		`emails[type eq "work"]`,
		`emails[type eq "work" and value co "@example.com"]`,
		`userType eq "Employee" and (emails co "example.com" or emails.value co "example.org")`,
		`userType ne "Employee" and not (emails co "example.com" or emails.value co "example.org")`,
		`userType eq "Employee" and (emails.type eq "work")`,
		`schemas eq "urn:ietf:params:scim:schemas:core:2.0:User"`,
		`urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber eq "701984"`,
	}
	for _, f := range valid {
		_, err := Parse(f)
		require.NoError(t, err, f)
	}

	malformed := []string{
		`userName eq`,
		`name..familyName eq "x"`,
		`(userName eq "x"`,
	}
	for _, f := range malformed {
		_, err := Parse(f)
		require.ErrorIs(t, err, ErrInvalidFilter, f)
	}

	long := `userName eq "` + strings.Repeat("a", defaultGrammar.maxInputBytes) + `"`
	_, err := Parse(long)
	require.ErrorIs(t, err, ErrInputTooLarge, long)
}
