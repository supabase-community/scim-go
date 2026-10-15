package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/scimtest"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestConformance(t *testing.T) {
	for name, tc := range map[string]struct {
		config *core.ServiceProviderConfig
		opts   []scimtest.Option[*scimtest.Suite]
	}{
		"full strict":    {fullServiceProviderConfig(), []scimtest.Option[*scimtest.Suite]{scimtest.Strict()}},
		"minimal strict": {core.NewServiceProviderConfig(), []scimtest.Option[*scimtest.Suite]{scimtest.Strict()}},
		"minimal":        {core.NewServiceProviderConfig(), nil},
	} {
		t.Run(name, func(t *testing.T) {
			handler := server.New(basePath, tc.config,
				server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).
					WithExtension(core.SchemaEnterpriseUser, core.EnterpriseUserAttributes()...)),
				server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...)),
				server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
			)
			c := scimtest.Serve(t, handler, basePath, scimtest.WithBearerToken(validToken))
			scimtest.NewSuite(c, tc.opts...).Run(t)
		})
	}
}

func TestConformanceWithFixture(t *testing.T) {
	inner := server.New(basePath, fullServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
	)
	emailOnly := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var user core.User
		if (r.Method == http.MethodPost || r.Method == http.MethodPut) && json.Unmarshal(body, &user) == nil && !strings.Contains(user.UserName, "@") {
			_ = protocol.SendError(w, scimerrors.NewError(http.StatusBadRequest, scimerrors.InvalidValue, "userName must be an email"))
			return
		}
		inner.ServeHTTP(w, r)
	})

	scimtest.NewSuite(scimtest.Serve(t, emailOnly, basePath),
		scimtest.WithFixture("User", func(unique string) *core.User {
			return &core.User{UserName: "bjensen+" + unique + "@example.com"}
		}),
	).Run(t)
}
