package memory

import (
	"slices"
	"strconv"

	"github.com/supabase-community/scim-go/internal/idset"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/store"
)

type entries map[string]entry

func (es entries) find(id string, schema *core.Schema) (entry, bool) {
	e, ok := es[id]
	return e, ok && e.schema == schema
}

func (es entries) idsOf(schema *core.Schema) []string {
	ids := []string{}
	for id, e := range es {
		if e.schema == schema {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func (es entries) checkLinks(id string, before, links []store.Link) error {
	for _, link := range links {
		add, _ := store.LinkDiff(targetsOf(before, link.Attribute), link.Targets)
		if err := es.checkTargets(link, add); err != nil {
			return err
		}
		if link.Soft {
			continue
		}
		if err := es.checkAcyclic(id, link.Attribute, add); err != nil {
			return err
		}
	}
	return nil
}

// RFC 7643 Section 2.3.7: a reference value is the id of an existing resource, except for a soft link, which may name an outside resource.
func (es entries) checkTargets(link store.Link, ids []string) error {
	for _, id := range ids {
		target, ok := es[id]
		if !ok && link.Soft {
			continue
		}
		if !ok || !slices.Contains(link.Types, string(target.schema.Name)) {
			return scimerrors.ErrInvalidValue(strconv.Quote(id) + " does not reference an existing resource")
		}
	}
	return nil
}

func (es entries) checkAcyclic(source, attribute string, targets []string) error {
	for _, target := range targets {
		if es.reaches(target, source, attribute) {
			return scimerrors.ErrInvalidValue(strconv.Quote(target) + " would make " + attribute + " cyclic")
		}
	}
	return nil
}

func (es entries) reaches(from, goal, attribute string) bool {
	seen := idset.Set{}
	pending := []string{from}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if id == goal {
			return true
		}
		if !seen.Has(id) {
			seen.Add(id)
			pending = append(pending, targetsOf(es[id].links, attribute)...)
		}
	}
	return false
}

func (es entries) resolved(links []store.Link) []store.Link {
	resolved := make([]store.Link, len(links))
	for i, link := range links {
		link.Targets = slices.DeleteFunc(slices.Clone(link.Targets), func(id string) bool {
			_, held := es[id]
			return link.Soft && !held
		})
		resolved[i] = link
	}
	return resolved
}

// RFC 7232 Section 2.1: a weak entity tag SHOULD change when the representation changes, and groups is part of the User.
func (es entries) touch(ids []string) {
	for _, id := range ids {
		if e, ok := es[id]; ok {
			e.updated = bumped(e.updated)
			es[id] = e
		}
	}
}
