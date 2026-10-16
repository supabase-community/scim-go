package store

import "github.com/supabase-community/scim-go/internal/idset"

type Link struct {
	Attribute string
	Types     []string
	Targets   []string
	Soft      bool
}

func LinkDiff(current, wanted []string) (add, remove []string) {
	return missing(wanted, current), missing(current, wanted)
}

func missing(from, in []string) []string {
	var absent []string
	have, seen := idset.Set{}, idset.Set{}
	for _, id := range in {
		have.Add(id)
	}
	for _, id := range from {
		if !have.Has(id) && !seen.Has(id) {
			absent = append(absent, id)
		}
		seen.Add(id)
	}
	return absent
}
