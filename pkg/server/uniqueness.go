package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func UniquenessValidator[T core.Resource](repo Repository[T], schema *core.Schema) Validator[T] {
	attrs := uniqueAttributes(schema.Attributes)

	return func(ctx context.Context, candidate T, excludeID string) error {
		if len(attrs) == 0 {
			return nil
		}

		doc, err := toDoc(candidate)
		if err != nil {
			return err
		}

		items, _, err := repo.List(ctx, &protocol.SearchRequest{
			StartIndex: 1,
			Count:      maxListCount,
		})
		if err != nil {
			return err
		}

		for _, attr := range attrs {
			segments := strings.Split(strings.ToLower(attr.Name), ".")
			values := lookup(doc, segments)
			if len(values) == 0 {
				continue
			}
			for _, existing := range items {
				if existing.ResourceID() == excludeID {
					continue
				}
				existingDoc, err := toDoc(existing)
				if err != nil {
					return err
				}
				for _, existingValue := range lookup(existingDoc, segments) {
					for _, value := range values {
						if equalValues(attr, existingValue, value) {
							return scimerrors.ErrUniqueness(fmt.Sprintf("%q is already in use", attr.Name))
						}
					}
				}
			}
		}
		return nil
	}
}

func uniqueAttributes(attrs core.Attributes) []*core.Attribute {
	var unique []*core.Attribute
	for _, attr := range attrs {
		if attr.Uniqueness != core.UniquenessNone {
			unique = append(unique, attr)
		}
		unique = append(unique, uniqueAttributes(attr.SubAttributes)...)
	}
	return unique
}
