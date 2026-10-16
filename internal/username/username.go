package username

import (
	"errors"
	"strings"

	"golang.org/x/text/secure/bidirule"
	"golang.org/x/text/secure/precis"
	"golang.org/x/text/unicode/bidi"
	"golang.org/x/text/unicode/norm"
)

var (
	errEmptyUserpart = errors.New("username: empty userpart")
	caseMapped       = precis.NewIdentifier(precis.FoldWidth, precis.LowerCase(), precis.Norm(norm.NFC))
	casePreserved    = precis.NewIdentifier(precis.FoldWidth, precis.Norm(norm.NFC))
)

// Prepare enforces UsernameCasePreserved when caseExact, otherwise UsernameCaseMapped, per RFC 8265, Sections 3.3 and 3.4.
func Prepare(s string, caseExact bool) (string, error) {
	profile := caseMapped
	if caseExact {
		profile = casePreserved
	}
	// RFC 8265 Section 3.1: username = userpart *(1*SP userpart)
	if s == "" || strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") {
		return "", errEmptyUserpart
	}
	parts := strings.Split(s, " ")
	for i, part := range parts {
		if part == "" {
			continue
		}
		prepared, err := enforce(profile, part)
		if err != nil {
			return "", err
		}
		parts[i] = prepared
	}
	return strings.Join(parts, " "), nil
}

// RFC 8265 Section 3.3.2: the Bidi Rule applies only to strings that contain right-to-left code points.
func enforce(profile *precis.Profile, part string) (string, error) {
	prepared, err := profile.String(part)
	if err != nil {
		return "", err
	}
	if bidirule.DirectionString(prepared) == bidi.RightToLeft && !bidirule.ValidString(prepared) {
		return "", bidirule.ErrInvalid
	}
	return prepared, nil
}
