package core_test

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimtest"
)

func TestRFC7643(t *testing.T) {
	t.Run("carries the minimal User of Section 8.1 whole", func(t *testing.T) {
		assert.Empty(t, scimtest.RoundTripDiff(t, scimtest.RFC7643MinimalUser, &core.User{}))
	})

	t.Run("carries the full User of Section 8.2 whole", func(t *testing.T) {
		assert.Empty(t, scimtest.RoundTripDiff(t, scimtest.RFC7643FullUser, &core.User{}))
	})

	t.Run("carries the enterprise extension of Section 8.3 whole", func(t *testing.T) {
		assert.Empty(t, scimtest.RoundTripDiff(t, scimtest.RFC7643EnterpriseUser, &core.User{}))
	})

	t.Run("carries the Group of Section 8.4 whole", func(t *testing.T) {
		assert.Empty(t, scimtest.RoundTripDiff(t, scimtest.RFC7643Group, &core.Group{}))
	})

	t.Run("carries the service provider configuration of Section 8.5 whole", func(t *testing.T) {
		assert.Empty(t, scimtest.RoundTripDiff(t, scimtest.RFC7643ServiceProviderConfiguration, &core.ServiceProviderConfig{}))
	})

	t.Run("carries the resource types of Section 8.6 whole", func(t *testing.T) {
		assert.Empty(t, scimtest.RoundTripDiff(t, scimtest.RFC7643ResourceTypes, &[]core.ResourceType{}))
	})

	// RFC 7643, Section 7 states an attribute characteristic only where it bears on the attribute.
	t.Run("states attribute characteristics the schemas of Section 8.7 leave unsaid", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			stray []string
		}{
			{scimtest.RFC7643ResourceSchemas, []string{"canonicalValues", "caseExact", "uniqueness"}},
			{scimtest.RFC7643ServiceProviderSchemas, []string{"caseExact", "uniqueness"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assert.Equal(t, tc.stray, leaves(scimtest.RoundTripDiff(t, tc.name, &[]core.Schema{})))
			})
		}
	})

	t.Run("declares the attributes of the resource schemas of Section 8.7.1", func(t *testing.T) {
		var schemas []core.Schema
		require.NoError(t, json.Unmarshal(scimtest.Golden(t, scimtest.RFC7643ResourceSchemas), &schemas))

		for _, tc := range []struct {
			schema     core.Schema
			attributes core.Attributes
			deviations []string
		}{
			{schemas[0], core.UserAttributes(), []string{"addresses.primary"}},
			{schemas[1], core.GroupAttributes(), []string{"displayName.required", "members.display"}},
			{schemas[2], core.EnterpriseUserAttributes(), nil},
		} {
			t.Run(string(tc.schema.ID), func(t *testing.T) {
				assert.Equal(t, tc.deviations, deviations("", tc.schema.Attributes, tc.attributes))
			})
		}
	})
}

func deviations(path string, want, got core.Attributes) []string {
	var paths []string
	for _, attribute := range got {
		name := path + attribute.Name
		expected := want.Lookup(attribute.Name)
		if expected == nil {
			paths = append(paths, name)
			continue
		}
		for _, characteristic := range []struct {
			name string
			same bool
		}{
			{"name", expected.Name == attribute.Name},
			{"type", expected.Type == attribute.Type},
			{"multiValued", expected.MultiValued == attribute.MultiValued},
			{"required", expected.Required == attribute.Required},
			{"caseExact", expected.CaseExact == attribute.CaseExact},
			{"mutability", expected.Mutability == attribute.Mutability},
			{"returned", expected.Returned == attribute.Returned},
			{"uniqueness", expected.Uniqueness == "" || expected.Uniqueness == attribute.Uniqueness},
			{"canonicalValues", slices.Equal(expected.CanonicalValues, attribute.CanonicalValues)},
			{"referenceTypes", slices.Equal(expected.ReferenceTypes, attribute.ReferenceTypes)},
		} {
			if !characteristic.same {
				paths = append(paths, name+"."+characteristic.name)
			}
		}
		paths = append(paths, deviations(name+".", expected.SubAttributes, attribute.SubAttributes)...)
	}
	for _, attribute := range want {
		if got.Lookup(attribute.Name) == nil {
			paths = append(paths, path+attribute.Name+".missing")
		}
	}
	return paths
}

func leaves(paths []string) []string {
	seen := map[string]bool{}
	for _, path := range paths {
		parts := strings.Split(path, ".")
		last := parts[len(parts)-1]
		if idx := strings.Index(last, "["); idx >= 0 {
			last = last[:idx]
		}
		seen[last] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
