package core

import (
	"testing"
	"time"

	"github.com/supabase-community/go-scim/pkg/scimtest"
)

var (
	createdAt = time.Date(2010, 1, 23, 4, 56, 22, 0, time.UTC)
	updatedAt = time.Date(2011, 5, 13, 4, 42, 34, 0, time.UTC)
)

func TestAssertJSON(t *testing.T) {
	t.Run("builds the minimal User of Section 8.1", func(t *testing.T) {
		user := &User{
			CommonAttributes: CommonAttributes{
				Schemas: []SchemaURI{SchemaUser},
				ID:      "2819c223-7f76-453a-919d-413861904646",
				Meta: Meta{
					ResourceType: "User",
					Created:      createdAt,
					LastModified: updatedAt,
					Version:      `W/"3694e05e9dff590"`,
					Location:     "https://example.com/v2/Users/2819c223-7f76-453a-919d-413861904646",
				},
			},
			UserName: "bjensen@example.com",
		}

		scimtest.AssertJSON(t, scimtest.RFC7643MinimalUser, user)
	})

	t.Run("builds the full User of Section 8.2", func(t *testing.T) {
		scimtest.AssertJSON(t, scimtest.RFC7643FullUser, fullUser(), "password")
	})

	t.Run("builds the enterprise User of Section 8.3", func(t *testing.T) {
		user := fullUser()
		user.Schemas = []SchemaURI{
			SchemaUser,
			SchemaEnterpriseUser,
		}
		user.Name.Formatted = "Ms. Barbara J Jensen, III"
		user.Meta.Version = `W/"3694e05e9dff591"`
		user.Groups = []GroupMembership{
			{
				Value:   "e9e30dba-f08f-4109-8486-d5c6a331660a",
				Ref:     "../Groups/e9e30dba-f08f-4109-8486-d5c6a331660a",
				Display: "Tour Guides",
			},
			{
				Value:   "fc348aa8-3835-40eb-a20b-c726e15c55b5",
				Ref:     "../Groups/fc348aa8-3835-40eb-a20b-c726e15c55b5",
				Display: "Employees",
			},
			{
				Value:   "71ddacd2-a8e7-49b8-a5db-ae50d0a5bfd7",
				Ref:     "../Groups/71ddacd2-a8e7-49b8-a5db-ae50d0a5bfd7",
				Display: "US Employees",
			},
		}
		user.EnterpriseUser = &EnterpriseUser{
			EmployeeNumber: "701984",
			CostCenter:     "4130",
			Organization:   "Universal Studios",
			Division:       "Theme Park",
			Department:     "Tour Operations",
			Manager: &Manager{
				Value:       "26118915-6090-4610-87e4-49d8ca9f808d",
				Ref:         "../Users/26118915-6090-4610-87e4-49d8ca9f808d",
				DisplayName: "John Smith",
			},
		}

		scimtest.AssertJSON(t, scimtest.RFC7643EnterpriseUser, user, "password")
	})

	t.Run("builds the Group of Section 8.4", func(t *testing.T) {
		group := &Group{
			CommonAttributes: CommonAttributes{
				Schemas: []SchemaURI{SchemaGroup},
				ID:      "e9e30dba-f08f-4109-8486-d5c6a331660a",
				Meta: Meta{
					ResourceType: "Group",
					Created:      createdAt,
					LastModified: updatedAt,
					Version:      `W/"3694e05e9dff592"`,
					Location:     "https://example.com/v2/Groups/e9e30dba-f08f-4109-8486-d5c6a331660a",
				},
			},
			DisplayName: "Tour Guides",
			Members: []Member{
				{
					Value:   "2819c223-7f76-453a-919d-413861904646",
					Ref:     "https://example.com/v2/Users/2819c223-7f76-453a-919d-413861904646",
					Display: "Babs Jensen",
				},
				{
					Value:   "902c246b-6245-4190-8e05-00816be7344a",
					Ref:     "https://example.com/v2/Users/902c246b-6245-4190-8e05-00816be7344a",
					Display: "Mandy Pepperidge",
				},
			},
		}

		scimtest.AssertJSON(t, scimtest.RFC7643Group, group)
	})

	t.Run("builds the service provider configuration of Section 8.5", func(t *testing.T) {
		oauth := NewOAuthBearerToken().AsPrimary()
		oauth.DocumentationURI = "http://example.com/help/oauth.html"

		config := &ServiceProviderConfig{
			Schemas:          []SchemaURI{SchemaServiceProviderConfig},
			DocumentationURI: "http://example.com/help/scim.html",
			Bulk:             BulkFeature{Supported: true, MaxOperations: 1000, MaxPayloadSize: 1048576},
			ChangePassword:   SupportedFeature{Supported: true},
			ETag:             SupportedFeature{Supported: true},
			AuthenticationSchemes: []*AuthenticationScheme{
				oauth,
				{
					Type:             AuthenticationSchemeHTTPBasic,
					Name:             "HTTP Basic",
					Description:      "Authentication scheme using the HTTP Basic Standard",
					SpecURI:          "http://www.rfc-editor.org/info/rfc2617",
					DocumentationURI: "http://example.com/help/httpBasic.html",
				},
			},
			Meta: Meta{
				ResourceType: "ServiceProviderConfig",
				Created:      createdAt,
				LastModified: updatedAt,
				Version:      `W/"3694e05e9dff594"`,
				Location:     "https://example.com/v2/ServiceProviderConfig",
			},
		}
		config.Patching().Sorting().Filtering(200)

		scimtest.AssertJSON(t, scimtest.RFC7643ServiceProviderConfiguration, config)
	})

	t.Run("builds the resource types of Section 8.6", func(t *testing.T) {
		userType := userResourceType()

		groupType := &ResourceType{
			Schemas:     []SchemaURI{SchemaResourceType},
			ID:          "Group",
			Name:        "Group",
			Endpoint:    "/Groups",
			Description: "Group",
			Schema:      SchemaGroup,
			Meta: Meta{
				ResourceType: "ResourceType",
				Location:     "https://example.com/v2/ResourceTypes/Group",
			},
		}

		scimtest.AssertJSON(t, scimtest.RFC7643ResourceTypes, []*ResourceType{userType, groupType})
	})
}

func fullUser() *User {
	return &User{
		CommonAttributes: CommonAttributes{
			Schemas: []SchemaURI{SchemaUser},
			ID:      "2819c223-7f76-453a-919d-413861904646",
			Meta: Meta{
				ResourceType: "User",
				Created:      createdAt,
				LastModified: updatedAt,
				Version:      `W/"a330bc54f0671c9"`,
				Location:     "https://example.com/v2/Users/2819c223-7f76-453a-919d-413861904646",
			},
		},
		ExternalID: "701984",
		UserName:   "bjensen@example.com",
		Name: Name{
			Formatted:       "Ms. Barbara J Jensen, III",
			FamilyName:      "Jensen",
			GivenName:       "Barbara",
			MiddleName:      "Jane",
			HonorificPrefix: "Ms.",
			HonorificSuffix: "III",
		},
		DisplayName: "Babs Jensen",
		NickName:    "Babs",
		ProfileURL:  "https://login.example.com/bjensen",
		Emails: []Email{
			{
				Value:   "bjensen@example.com",
				Type:    "work",
				Primary: new(true),
			},
			{
				Value: "babs@jensen.org",
				Type:  "home",
			},
		},
		Addresses: []Address{
			{
				Type:          "work",
				StreetAddress: "100 Universal City Plaza",
				Locality:      "Hollywood",
				Region:        "CA",
				PostalCode:    "91608",
				Country:       "USA",
				Formatted:     "100 Universal City Plaza\nHollywood, CA 91608 USA",
				Primary:       true,
			},
			{
				Type:          "home",
				StreetAddress: "456 Hollywood Blvd",
				Locality:      "Hollywood",
				Region:        "CA",
				PostalCode:    "91608",
				Country:       "USA",
				Formatted:     "456 Hollywood Blvd\nHollywood, CA 91608 USA",
			},
		},
		PhoneNumbers: []PhoneNumber{
			{
				Value: "555-555-5555",
				Type:  "work",
			},
			{
				Value: "555-555-4444",
				Type:  "mobile",
			},
		},
		IMS: []IM{
			{
				Value: "someaimhandle",
				Type:  "aim",
			},
		},
		Photos: []Photo{
			{
				Value: "https://photos.example.com/profilephoto/72930000000Ccne/F",
				Type:  "photo",
			},
			{
				Value: "https://photos.example.com/profilephoto/72930000000Ccne/T",
				Type:  "thumbnail",
			},
		},
		UserType:          "Employee",
		Title:             "Tour Guide",
		PreferredLanguage: "en-US",
		Locale:            "en-US",
		Timezone:          "America/Los_Angeles",
		Active:            new(true),
		Password:          "t1meMa$heen",
		Groups: []GroupMembership{
			{
				Value:   "e9e30dba-f08f-4109-8486-d5c6a331660a",
				Ref:     "https://example.com/v2/Groups/e9e30dba-f08f-4109-8486-d5c6a331660a",
				Display: "Tour Guides",
			},
			{
				Value:   "fc348aa8-3835-40eb-a20b-c726e15c55b5",
				Ref:     "https://example.com/v2/Groups/fc348aa8-3835-40eb-a20b-c726e15c55b5",
				Display: "Employees",
			},
			{
				Value:   "71ddacd2-a8e7-49b8-a5db-ae50d0a5bfd7",
				Ref:     "https://example.com/v2/Groups/71ddacd2-a8e7-49b8-a5db-ae50d0a5bfd7",
				Display: "US Employees",
			},
		},
		X509Certificates: []X509Certificate{
			{
				Value: "MIIDQzCCAqygAwIBAgICEAAwDQYJKoZIhvcNAQEFBQAwTjELMAkGA1UEBhMCVVMx EzARBgNVBAgMCkNhbGlmb3JuaWExFDASBgNVBAoMC2V4YW1wbGUuY29tMRQwEgYD VQQDDAtleGFtcGxlLmNvbTAeFw0xMTEwMjIwNjI0MzFaFw0xMjEwMDQwNjI0MzFa MH8xCzAJBgNVBAYTAlVTMRMwEQYDVQQIDApDYWxpZm9ybmlhMRQwEgYDVQQKDAtl eGFtcGxlLmNvbTEhMB8GA1UEAwwYTXMuIEJhcmJhcmEgSiBKZW5zZW4gSUlJMSIw IAYJKoZIhvcNAQkBFhNiamVuc2VuQGV4YW1wbGUuY29tMIIBIjANBgkqhkiG9w0B AQEFAAOCAQ8AMIIBCgKCAQEA7Kr+Dcds/JQ5GwejJFcBIP682X3xpjis56AK02bc 1FLgzdLI8auoR+cC9/Vrh5t66HkQIOdA4unHh0AaZ4xL5PhVbXIPMB5vAPKpzz5i PSi8xO8SL7I7SDhcBVJhqVqr3HgllEG6UClDdHO7nkLuwXq8HcISKkbT5WFTVfFZ zidPl8HZ7DhXkZIRtJwBweq4bvm3hM1Os7UQH05ZS6cVDgweKNwdLLrT51ikSQG3 DYrl+ft781UQRIqxgwqCfXEuDiinPh0kkvIi5jivVu1Z9QiwlYEdRbLJ4zJQBmDr SGTMYn4lRc2HgHO4DqB/bnMVorHB0CC6AV1QoFK4GPe1LwIDAQABo3sweTAJBgNV HRMEAjAAMCwGCWCGSAGG+EIBDQQfFh1PcGVuU1NMIEdlbmVyYXRlZCBDZXJ0aWZp Y2F0ZTAdBgNVHQ4EFgQU8pD0U0vsZIsaA16lL8En8bx0F/gwHwYDVR0jBBgwFoAU dGeKitcaF7gnzsNwDx708kqaVt0wDQYJKoZIhvcNAQEFBQADgYEAA81SsFnOdYJt Ng5Tcq+/ByEDrBgnusx0jloUhByPMEVkoMZ3J7j1ZgI8rAbOkNngX8+pKfTiDz1R C4+dx8oU6Za+4NJXUjlL5CvV6BEYb1+QAEJwitTVvxB/A67g42/vzgAtoRUeDov1 +GFiBZ+GNF/cAYKcMtGcrs2i97ZkJMo=",
			},
		},
	}
}
