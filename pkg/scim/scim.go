package scim

import (
	"context"
	"strings"

	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
)

// BasePath is the mount point of the SCIM service, per RFC 7644, Section 3.2.
const BasePath = "/scim/v2"

func Join(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(segment, "/")
}

func NewServiceProviderConfig(baseURL string, schemes ...*core.AuthenticationScheme) *core.ServiceProviderConfig {
	if schemes == nil {
		schemes = []*core.AuthenticationScheme{}
	}
	return &core.ServiceProviderConfig{
		Schemas:               []core.SchemaURI{core.SchemaServiceProviderConfig},
		AuthenticationSchemes: schemes,
		Meta: core.Meta{
			ResourceType: "ServiceProviderConfig",
			Location:     Join(baseURL, "/ServiceProviderConfig"),
		},
	}
}

type Repository[T core.Resource] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, id string, item T) (T, error)
	Delete(ctx context.Context, id string) error
}
