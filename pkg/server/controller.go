package server

import (
	"net/http"
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Controller handles the HTTP requests for one SCIM resource type, per RFC 7644, Section 3.
type Controller[T core.Resource] interface {
	List(http.ResponseWriter, *http.Request) error
	Search(http.ResponseWriter, *http.Request) error
	ByID(http.ResponseWriter, *http.Request) error
	Create(http.ResponseWriter, *http.Request) error
	Replace(http.ResponseWriter, *http.Request) error
	Patch(http.ResponseWriter, *http.Request) error
	Delete(http.ResponseWriter, *http.Request) error
}

type controller[T core.Resource] struct {
	schemas core.Schemas
	service Service[T]
	limits  Limits
	config  *core.ServiceProviderConfig
}

func NewController[T core.Resource](service Service[T], schemas core.Schemas, limits Limits, config *core.ServiceProviderConfig) Controller[T] {
	return &controller[T]{
		schemas: schemas,
		service: service,
		limits:  limits,
		config:  config,
	}
}

func (c *controller[T]) List(w http.ResponseWriter, r *http.Request) error {
	query, err := c.limits.ParseSearchRequest(r.URL.Query())
	if err != nil {
		return protocol.SendError(w, err)
	}
	return c.list(w, r, query)
}

// Search queries resources with POST ".search", per RFC 7644, Section 3.4.3.
func (c *controller[T]) Search(w http.ResponseWriter, r *http.Request) error {
	query, err := c.limits.DecodeSearchRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	return c.list(w, r, query)
}

func (c *controller[T]) list(w http.ResponseWriter, r *http.Request, query *protocol.SearchRequest) error {
	if query.Filter != "" && !c.config.SupportsFilter() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"filter" is not supported`))
	}
	if query.SortBy != "" && !c.config.SupportsSort() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"sortBy" is not supported`))
	}
	if err := query.Validate(c.schemas); err != nil {
		return protocol.SendError(w, err)
	}
	projection, err := query.Projection(c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	items, total, err := c.service.List(protocol.WithProjection(r.Context(), projection), query)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resources := projection.All(items)
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(query.StartIndex, total, resources))
}

func (c *controller[T]) ByID(w http.ResponseWriter, r *http.Request) error {
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	resource, ok, err := c.existing(w, r.WithContext(protocol.WithProjection(r.Context(), projection)))
	if !ok {
		return err
	}
	c.setVersion(w, resource)
	return c.send(w, http.StatusOK, resource, projection)
}

func (c *controller[T]) Create(w http.ResponseWriter, r *http.Request) error {
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	document, err := protocol.DecodeDocument(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	projection = projection.Written(document)
	created, err := c.service.Create(r.Context(), document)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("Location", created.Common().Meta.Location)
	c.setVersion(w, created)
	return c.send(w, http.StatusCreated, created, projection)
}

func (c *controller[T]) Replace(w http.ResponseWriter, r *http.Request) error {
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	req, err := protocol.DecodeReplaceRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	versions, err := c.ifMatch(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	req.ID, req.Versions = r.PathValue("id"), versions
	projection = projection.Written(req.Attributes)
	replaced, err := c.service.Replace(r.Context(), req)
	if err != nil {
		return protocol.SendError(w, err)
	}
	c.setVersion(w, replaced)
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) Patch(w http.ResponseWriter, r *http.Request) error {
	if !c.config.SupportsPatch() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"PATCH" is not supported`))
	}
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	req, err := c.limits.DecodePatchRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	versions, err := c.ifMatch(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	req.ID, req.Versions = r.PathValue("id"), versions
	projection = projection.Patched(req.Operations)
	patched, err := c.service.Patch(r.Context(), req)
	if err != nil {
		return protocol.SendError(w, err)
	}
	c.setVersion(w, patched)
	return c.sendPatched(w, patched, projection, noContentEligible(r, c.schemas, req.Operations))
}

// sendPatched returns 204 for a Group PATCH eligible under noContentEligible, or 200 with the resource otherwise, per RFC 7644 Section 3.5.2.
func (c *controller[T]) sendPatched(w http.ResponseWriter, patched T, projection protocol.Projection, noContent bool) error {
	if noContent {
		setLocation(w, patched.Common().Meta)
		return protocol.Send(w, http.StatusNoContent, nil)
	}
	return c.send(w, http.StatusOK, patched, projection)
}

func (c *controller[T]) Delete(w http.ResponseWriter, r *http.Request) error {
	versions, err := c.ifMatch(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := c.service.Delete(r.Context(), &protocol.DeleteRequest{ID: r.PathValue("id"), Versions: versions}); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}

func (c *controller[T]) projectionFor(w http.ResponseWriter, r *http.Request) (projection protocol.Projection, ok bool, err error) {
	projection, err = protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return projection, false, protocol.SendError(w, err)
	}
	return projection, true, nil
}

func (c *controller[T]) existing(w http.ResponseWriter, r *http.Request) (resource T, ok bool, err error) {
	resource, err = c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return resource, false, protocol.SendError(w, err)
	}
	return resource, true, nil
}

func (c *controller[T]) send(w http.ResponseWriter, status int, resource T, projection protocol.Projection) error {
	setLocation(w, resource.Common().Meta)
	return protocol.Send(w, status, projection.Of(resource))
}

func (c *controller[T]) setVersion(w http.ResponseWriter, resource T) {
	if c.config.SupportsVersioning() {
		w.Header().Set("ETag", resource.Common().Meta.Version)
	}
}

// RFC 9110 Section 13.1.1: an If-Match list with no entity-tag never matches, so the condition is false.
func (c *controller[T]) ifMatch(r *http.Request) ([]string, error) {
	lines := r.Header.Values("If-Match")
	if !c.config.SupportsVersioning() || slices.Equal(lines, []string{"*"}) {
		return nil, nil
	}
	tags := entityTags(strings.Join(lines, ","))
	if len(lines) > 0 && len(tags) == 0 {
		return nil, scimerrors.ErrPreconditionFailed("If-Match lists no entity-tag")
	}
	return tags, nil
}

// RFC 7232 Section 3.1: If-Match = "*" / 1#entity-tag
func entityTags(field string) []string {
	var tags []string
	for field = strings.TrimLeft(field, " \t,"); field != ""; field = strings.TrimLeft(field, " \t,") {
		end := tagEnd(field)
		tags = append(tags, field[:end])
		field = field[end:]
	}
	return tags
}

func tagEnd(field string) int {
	opaque := strings.TrimPrefix(field, "W/")
	if strings.HasPrefix(opaque, `"`) {
		if end := strings.IndexByte(opaque[1:], '"'); end >= 0 {
			return len(field) - len(opaque) + end + 2
		}
	}
	if end := strings.IndexByte(field, ','); end >= 0 {
		return end
	}
	return len(field)
}
