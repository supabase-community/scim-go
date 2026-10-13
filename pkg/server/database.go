package server

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type encodedValue string

type record struct {
	core.Resource
	object  core.Object
	schemas core.Schemas
}

type indexKey struct {
	scope     *core.Schema
	attribute *core.Attribute
	value     any
}

type database struct {
	mu          sync.Mutex
	resources   map[string]core.Resource
	references  map[string]map[string]struct{}
	uniqueIndex map[indexKey]string
}

func newDatabase() *database {
	return &database{
		resources:   map[string]core.Resource{},
		references:  map[string]map[string]struct{}{},
		uniqueIndex: map[indexKey]string{},
	}
}

func (d *database) all(scope core.Schemas) []record {
	d.mu.Lock()
	defer d.mu.Unlock()
	records := []record{}
	for _, resource := range d.resources {
		if rec := resource.(record); inScope(rec, scope) {
			records = append(records, rec)
		}
	}
	slices.SortFunc(records, func(a, b record) int { return strings.Compare(a.Common().ID, b.Common().ID) })
	return records
}

func (d *database) find(id string, scope core.Schemas) (core.Resource, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, err := d.locate(id, "", scope)
	if err != nil {
		return nil, err
	}
	return clone(rec.Resource), nil
}

func (d *database) create(item core.Resource, scope core.Schemas, unique fields) (core.Resource, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.store(item, scope, unique)
}

func (d *database) update(item core.Resource, scope core.Schemas, unique fields) (core.Resource, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	common := item.Common()
	current, err := d.locate(common.ID, common.Meta.Version, scope)
	if err != nil {
		return nil, err
	}
	common.Meta = bumpMeta(current.Common().Meta)
	return d.store(item, scope, unique)
}

func (d *database) store(item core.Resource, scope core.Schemas, unique fields) (core.Resource, error) {
	rec, err := d.derive(item, scope)
	if err != nil {
		return nil, err
	}
	keys := indexKeys(rec, unique)
	if attribute := d.conflict(rec, keys); attribute != nil {
		return nil, scimerrors.ErrUniqueness(strconv.Quote(attribute.Name) + " must be unique")
	}
	id := rec.Common().ID
	if old, ok := d.resources[id].(record); ok {
		d.unindex(indexKeys(old, unique))
	}
	touched := d.unlink(id)
	d.resources[id] = rec
	touched = append(touched, d.link(rec)...)
	d.index(keys, id)
	d.refresh(append(touched, d.dependents(id)...))
	return clone(d.resources[id]), nil
}

func (d *database) remove(id, version string, scope core.Schemas, unique fields) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	rec, err := d.locate(id, version, scope)
	if err != nil {
		return err
	}
	d.unindex(indexKeys(rec, unique))
	referrers := d.references[id]
	touched := append(d.unlink(id), d.dependents(id)...)
	delete(d.resources, id)
	delete(d.references, id)
	d.detach(referrers, id)
	d.refresh(touched)
	return nil
}

// RFC 7644 Section 3.14: a non-empty version must match the stored one.
func (d *database) locate(id, version string, scope core.Schemas) (record, error) {
	rec, ok := d.resources[id].(record)
	switch {
	case !ok || !inScope(rec, scope):
		return record{}, scimerrors.ErrNotFound("Not found")
	case version != "" && rec.Common().Meta.Version != version:
		return record{}, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	return rec, nil
}

// RFC 7643 Section 7: a unique value may be held by only one resource.
func (d *database) conflict(candidate record, keys []indexKey) *core.Attribute {
	for _, key := range keys {
		if id, ok := d.uniqueIndex[key]; ok && id != candidate.Common().ID {
			return key.attribute
		}
	}
	return nil
}

func (d *database) index(keys []indexKey, id string) {
	for _, key := range keys {
		d.uniqueIndex[key] = id
	}
}

func (d *database) unindex(keys []indexKey) {
	for _, key := range keys {
		delete(d.uniqueIndex, key)
	}
}

func (d *database) link(rec record) []string {
	members, pointers := targetsOf(rec)
	for _, target := range append(pointers, members...) {
		if d.references[target] == nil {
			d.references[target] = map[string]struct{}{}
		}
		d.references[target][rec.Common().ID] = struct{}{}
	}
	return members
}

func (d *database) unlink(source string) []string {
	rec, ok := d.resources[source].(record)
	if !ok {
		return nil
	}
	members, pointers := targetsOf(rec)
	for _, target := range append(pointers, members...) {
		delete(d.references[target], source)
		if len(d.references[target]) == 0 {
			delete(d.references, target)
		}
	}
	return members
}

func (d *database) dependents(target string) []string {
	ids := []string{}
	for source := range d.references[target] {
		if rec, ok := d.resources[source].(record); ok && len(pointerFields(rec.schemas)) > 0 {
			ids = append(ids, source)
		}
	}
	return ids
}

func (d *database) refresh(ids []string) {
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		rec, ok := d.resources[id].(record)
		if !ok || !derives(rec.schemas) {
			continue
		}
		d.rederive(clone(rec), rec.schemas)
	}
}

// RFC 7643 Section 2.3.7: a service provider MAY enforce referential integrity.
func (d *database) detach(sources map[string]struct{}, target string) {
	for source := range sources {
		rec, ok := d.resources[source].(record)
		if !ok {
			continue
		}
		item := clone(rec)
		object, err := core.NewObject(item)
		if err != nil || !dropMember(object, rec.schemas, target) {
			continue
		}
		if resource, err := decodeAs(item, object); err == nil {
			d.rederive(resource, rec.schemas)
		}
	}
}

func (d *database) rederive(item core.Resource, scope core.Schemas) {
	item.Common().Meta = bumpMeta(item.Common().Meta)
	if derived, err := d.derive(item, scope); err == nil {
		d.resources[item.Common().ID] = derived
	}
}

func (d *database) derive(item core.Resource, scope core.Schemas) (record, error) {
	object, err := core.NewObject(item)
	if err != nil || !derives(scope) {
		return record{Resource: clone(item), object: object, schemas: scope}, err
	}
	for _, field := range derivedFields(scope) {
		d.fill(object, field, item.Common().ID)
	}
	for _, field := range pointerFields(scope) {
		d.follow(object, field)
	}
	resource, err := decodeAs(item, object)
	if err != nil {
		return record{}, err
	}
	object, err = core.NewObject(resource)
	return record{Resource: resource, object: object, schemas: scope}, err
}

func (d *database) fill(object core.Object, field field, target string) {
	holder := value.AsObject(field.holder(object))
	if holder == nil {
		return
	}
	assign(holder, field.Name, d.referrers(field, target))
}

// RFC 7643 Section 4.1.2: groups is readOnly, derived from Group membership.
func (d *database) referrers(field field, target string) []any {
	types := refOf(field.Attribute).ReferenceTypes
	elements := []any{}
	for _, source := range slices.Sorted(maps.Keys(d.references[target])) {
		rec, ok := d.resources[source].(record)
		if !ok || !slices.Contains(types, rec.Common().Meta.ResourceType.Reference()) || len(sourceFields(rec.schemas)) == 0 {
			continue
		}
		elements = append(elements, map[string]any{
			"value":   source,
			"$ref":    rec.Common().Meta.Location,
			"display": rec.object.Get("displayName"),
			"type":    "direct",
		})
	}
	return elements
}

// RFC 7643 Section 4.3: manager.displayName is readOnly, derived from the manager.
func (d *database) follow(object core.Object, field field) {
	pointer := value.AsObject(field.raw(object))
	if pointer == nil {
		return
	}
	id, _ := pointer.Get("value").(string)
	target, ok := d.resources[id].(record)
	found := ok && slices.Contains(refOf(field.Attribute).ReferenceTypes, target.Common().Meta.ResourceType.Reference())
	for _, sub := range readOnlySubs(field.Attribute) {
		pointer.Remove(sub.Name)
		if found && !value.IsUnassigned(target.object.Get(sub.Name)) {
			pointer.Set(sub.Name, target.object.Get(sub.Name))
		}
	}
}

func inScope(rec record, scope core.Schemas) bool {
	return rec.schemas.Base() == scope.Base()
}

func clone(item core.Resource) core.Resource {
	if rec, ok := item.(record); ok {
		item = rec.Resource
	}
	original := reflect.ValueOf(item).Elem()
	copied := reflect.New(original.Type())
	copied.Elem().Set(original)
	return copied.Interface().(core.Resource)
}

func decodeAs(item core.Resource, object core.Object) (core.Resource, error) {
	raw, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	decoded := reflect.New(reflect.TypeOf(item).Elem())
	err = json.Unmarshal(raw, decoded.Interface())
	return decoded.Interface().(core.Resource), err
}

func targetsOf(rec record) (members, pointers []string) {
	for _, field := range sourceFields(rec.schemas) {
		members = append(members, idsOf(field.elements(rec.object))...)
	}
	for _, field := range pointerFields(rec.schemas) {
		pointers = append(pointers, idsOf([]any{field.raw(rec.object)})...)
	}
	return members, pointers
}

func dropMember(object core.Object, scope core.Schemas, target string) bool {
	dropped := false
	for _, field := range sourceFields(scope) {
		elements := field.elements(object)
		count := len(elements)
		kept := slices.DeleteFunc(elements, func(element any) bool {
			return value.AsObject(element).Get("value") == target
		})
		if len(kept) == count {
			continue
		}
		dropped = true
		assign(value.AsObject(field.holder(object)), field.Name, kept)
	}
	return dropped
}

func assign(holder core.Object, name string, elements []any) {
	if len(elements) == 0 {
		holder.Remove(name)
		return
	}
	holder.Set(name, elements)
}

func idsOf(elements []any) []string {
	ids := []string{}
	for _, element := range elements {
		if id, ok := value.AsObject(element).Get("value").(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func sourceFields(scope core.Schemas) fields {
	return referenceFields(scope, false)
}

func derivedFields(scope core.Schemas) fields {
	return referenceFields(scope, true)
}

func derives(scope core.Schemas) bool {
	return len(derivedFields(scope)) > 0 || len(pointerFields(scope)) > 0
}

func pointerFields(scope core.Schemas) fields {
	matching := fields{}
	for _, field := range newFields(scope) {
		if field.parent == nil && !field.MultiValued && field.Type == core.TypeComplex && refOf(field.Attribute) != nil && len(readOnlySubs(field.Attribute)) > 0 {
			matching = append(matching, field)
		}
	}
	return matching
}

func readOnlySubs(attribute *core.Attribute) []*core.Attribute {
	subs := []*core.Attribute{}
	for _, sub := range attribute.SubAttributes {
		if sub.Mutability == core.MutabilityReadOnly {
			subs = append(subs, sub)
		}
	}
	return subs
}

func referenceFields(scope core.Schemas, readOnly bool) fields {
	matching := fields{}
	for _, field := range newFields(scope) {
		if field.isList() && field.Type == core.TypeComplex && refOf(field.Attribute) != nil && (field.Mutability == core.MutabilityReadOnly) == readOnly {
			matching = append(matching, field)
		}
	}
	return matching
}

func refOf(attribute *core.Attribute) *core.Attribute {
	for _, sub := range attribute.SubAttributes {
		if sub.Name == "$ref" {
			return sub
		}
	}
	return nil
}

func indexKeys(rec record, unique fields) []indexKey {
	keys := []indexKey{}
	for _, field := range unique {
		if field.Uniqueness == core.UniquenessNone {
			continue
		}
		for _, v := range uniqueKeys(field, rec.object) {
			keys = append(keys, indexKey{scope: rec.schemas.Base(), attribute: field.Attribute, value: v})
		}
	}
	return keys
}

func uniqueKeys(field field, object core.Object) []any {
	keys := []any{}
	for _, v := range field.values(object) {
		if !value.IsUnassigned(v) {
			keys = append(keys, uniqueKey(field.Attribute, v))
		}
	}
	return keys
}

func uniqueKey(attribute *core.Attribute, v any) any {
	switch folded := value.Fold(attribute, v).(type) {
	case string, int64, float64, bool:
		return folded
	case time.Time:
		return folded.UTC()
	}
	encoded, _ := json.Marshal(v)
	return encodedValue(encoded)
}
