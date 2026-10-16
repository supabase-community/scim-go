package memory

import (
	"maps"
	"slices"

	"github.com/supabase-community/scim-go/internal/etag"
	"github.com/supabase-community/scim-go/internal/fields"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/store"
)

type query struct {
	*memoryStore
	request *protocol.SearchRequest
}

func (q query) run(references []store.Reference) ([]store.Record, int, error) {
	rows, err := q.matching()
	if err != nil {
		return nil, 0, err
	}
	return q.page(rows, references), len(rows), nil
}

func (q query) page(rows []row, references []store.Reference) []store.Record {
	start := min(q.request.Offset(), len(rows))
	end := start + min(max(q.request.Count, 0), len(rows)-start)
	records := make([]store.Record, 0, end-start)
	for _, r := range rows[start:end] {
		records = append(records, q.db.record(r.id, r.entry, references))
	}
	return records
}

func (q query) matching() ([]row, error) {
	rows := q.rows(q.request.Filter != "" || q.request.SortBy != "")
	if q.request.Filter != "" {
		matching, err := protocol.Filter(q.schemas, q.request.Filter, q.evaluator)
		if err != nil {
			return nil, err
		}
		rows = slices.DeleteFunc(rows, func(r row) bool { return !matching(r.object) })
	}
	return q.sorted(rows)
}

func (q query) rows(withObjects bool) []row {
	ids := q.db.entries.idsOf(q.schemas.Base())
	rows := make([]row, 0, len(ids))
	for _, id := range ids {
		r := row{id: id, entry: q.db.entries[id]}
		if withObjects {
			r.object = q.object(id, r.entry)
		}
		rows = append(rows, r)
	}
	return rows
}

func (q query) sorted(rows []row) ([]row, error) {
	if q.request.SortBy == "" {
		return rows, nil
	}
	attribute, key, err := q.sortBy()
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(rows, func(a, b row) int {
		return compareSortKeys(attribute, key(a.object), key(b.object), q.request.Descending())
	})
	return rows, nil
}

func (q query) sortBy() (*core.Attribute, func(core.Object) any, error) {
	target, err := q.request.SortAttribute(q.schemas)
	if err != nil {
		return nil, nil, err
	}
	key, ok := q.sortKey(target.Root(), target.Definition)
	if !ok {
		return nil, nil, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	return target.Definition, key, nil
}

// RFC 7644 Section 3.4.2.3: a multi-valued attribute sorts by its primary value, or else its first value.
func (q query) sortKey(parent, attribute *core.Attribute) (func(core.Object) any, bool) {
	list, ok := q.Lookup(parent)
	if !ok || !list.IsList() {
		field, ok := q.Lookup(attribute)
		return field.Value, ok
	}
	return func(d core.Object) any {
		element, ok := primaryOrFirst(list.Elements(d))
		if !ok {
			return nil
		}
		return fields.Coerce(attribute, value.AsObject(element).Get(attribute.Name))
	}, true
}

func (q query) object(id string, e entry) core.Object {
	object := q.cloned(e.body)
	object.Set("id", id)
	object.Set("meta", q.meta(id, e))
	return store.Nest(object, q.db.relations(id, e, q.references))
}

func (q query) cloned(body core.Object) core.Object {
	object := maps.Clone(body)
	for _, extension := range q.schemas.Extensions() {
		if holder := value.AsObject(object.Get(string(extension.ID))); holder != nil {
			object.Set(string(extension.ID), maps.Clone(holder))
		}
	}
	return object
}

func (q query) meta(id string, e entry) core.Object {
	meta, _ := core.NewObject(core.Meta{
		ResourceType: e.schema.Name,
		Created:      e.created,
		LastModified: e.updated,
		Location:     q.location + "/" + id,
		Version:      etag.Weak(e.updated),
	})
	return meta
}
