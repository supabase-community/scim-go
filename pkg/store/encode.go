package store

import (
	"encoding/json"
	"fmt"
	"strconv"
	"uuid"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const PasswordAttribute = "password"

func Encode[T core.Resource](item T, references []Reference) (Document, error) {
	object, err := core.NewObject(item)
	if err != nil {
		return Document{}, err
	}
	for _, key := range []string{"id", "meta", PasswordAttribute} {
		object.Remove(key)
	}
	links, err := extractLinks(item.Common().ID, object, references)
	if err != nil {
		return Document{}, err
	}
	body, err := json.Marshal(object)
	return Document{ID: item.Common().ID, Body: body, Links: links}, err
}

func extractLinks(id string, object core.Object, references []Reference) ([]Link, error) {
	links := []Link{}
	for _, reference := range references {
		holder := reference.holder(object, false)
		if !reference.Inverse() {
			link, err := newLink(id, reference, holder.Get(reference.Name))
			if err != nil {
				return nil, err
			}
			links = append(links, link)
		}
		keep(reference, holder)
	}
	return links, nil
}

// RFC 7643 Section 4.3: the server stores only the value of a manager, since displayName is readOnly.
func keep(reference Reference, holder core.Object) {
	pointer := value.AsObject(holder.Get(reference.Name))
	if !reference.Soft() || pointer == nil {
		holder.Remove(reference.Name)
		return
	}
	pointer.Remove("$ref")
	pointer.Remove("displayName")
}

func newLink(id string, reference Reference, raw any) (Link, error) {
	link := Link{Attribute: reference.Name, Types: reference.Types(), Soft: reference.Soft()}
	var err error
	if link.Soft {
		link.Targets = softTargets(id, raw)
	} else {
		link.Targets, err = hardTargets(reference.Name, raw)
	}
	return link, err
}

func softTargets(id string, raw any) []string {
	target, ok := value.Key(value.AsObject(raw))
	if _, err := uuid.Parse(target); !ok || err != nil || target == id {
		return nil
	}
	return []string{target}
}

func hardTargets(attribute string, raw any) ([]string, error) {
	elements, ok := raw.([]any)
	if raw != nil && !ok {
		return nil, invalidLink(attribute, fmt.Sprint(raw))
	}
	targets := make([]string, 0, len(elements))
	for _, element := range elements {
		id, _ := value.AsObject(element)[ValueAttribute].(string)
		if _, err := uuid.Parse(id); err != nil {
			return nil, invalidLink(attribute, id)
		}
		targets = append(targets, id)
	}
	return targets, nil
}

func invalidLink(attribute, id string) error {
	return scimerrors.ErrInvalidValue(strconv.Quote(id) + " is not a valid " + attribute + " value")
}
