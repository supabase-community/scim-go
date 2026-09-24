package core

type SupportedFeature struct {
	Supported bool `json:"supported"`
}

type BulkFeature struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

type FilterFeature struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

// ServiceProviderConfig is the schema defined in RFC 7643, Section 5.
type ServiceProviderConfig struct {
	basePath              string
	Schemas               []SchemaURI             `json:"schemas"`
	DocumentationURI      string                  `json:"documentationUri,omitempty"`
	Patch                 SupportedFeature        `json:"patch"`
	Bulk                  BulkFeature             `json:"bulk"`
	Filter                FilterFeature           `json:"filter"`
	ChangePassword        SupportedFeature        `json:"changePassword"`
	Sort                  SupportedFeature        `json:"sort"`
	ETag                  SupportedFeature        `json:"etag"`
	AuthenticationSchemes []*AuthenticationScheme `json:"authenticationSchemes"`
	Meta                  Meta                    `json:"meta"`
}

func NewServiceProviderConfig(basePath string) *ServiceProviderConfig {
	return &ServiceProviderConfig{
		basePath: basePath,
		Schemas:  []SchemaURI{SchemaServiceProviderConfig},
		Meta: Meta{
			ResourceType: "ServiceProviderConfig",
			Location:     basePath + "/ServiceProviderConfig",
		},
		AuthenticationSchemes: []*AuthenticationScheme{},
	}
}

// BasePath is the URL prefix this provider mounts its endpoints under.
func (c *ServiceProviderConfig) BasePath() string { return c.basePath }

func (c *ServiceProviderConfig) SupportsPatch() bool { return c.Patch.Supported }

func (c *ServiceProviderConfig) SupportsFilter() bool { return c.Filter.Supported }

func (c *ServiceProviderConfig) SupportsSort() bool { return c.Sort.Supported }

func (c *ServiceProviderConfig) SupportsVersioning() bool { return c.ETag.Supported }

// Sorting states that this provider honours "sortBy" and "sortOrder", per RFC 7644, Section 3.4.2.3.
func (c *ServiceProviderConfig) Sorting() *ServiceProviderConfig {
	c.Sort.Supported = true
	return c
}

// Filtering states that this provider honours "filter" up to maxResults, per RFC 7644, Section 3.4.2.2.
func (c *ServiceProviderConfig) Filtering(maxResults int) *ServiceProviderConfig {
	c.Filter.Supported = true
	c.Filter.MaxResults = maxResults
	return c
}

// Patching states that this provider honours the PATCH request of RFC 7644, Section 3.5.2.
func (c *ServiceProviderConfig) Patching() *ServiceProviderConfig {
	c.Patch.Supported = true
	return c
}

// Versioning states that this provider issues resource versions via ETags, per RFC 7644, Section 3.14.
func (c *ServiceProviderConfig) Versioning() *ServiceProviderConfig {
	c.ETag.Supported = true
	return c
}

// Authentication advertises the given schemes, per RFC 7643, Section 5.
func (c *ServiceProviderConfig) Authentication(schemes ...*AuthenticationScheme) *ServiceProviderConfig {
	c.AuthenticationSchemes = append(c.AuthenticationSchemes, schemes...)
	return c
}
