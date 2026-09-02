package core

// ServiceProviderConfig is the schema defined in RFC 7643, Section 5.
type ServiceProviderConfig struct {
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
