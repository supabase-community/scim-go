package memory

import "github.com/supabase-community/scim-go/pkg/store"

func changedTargets(before, links []store.Link) []string {
	changed := []string{}
	for _, link := range links {
		add, remove := store.LinkDiff(targetsOf(before, link.Attribute), link.Targets)
		changed = append(append(changed, add...), remove...)
	}
	return changed
}

func allTargets(links []store.Link) []string {
	all := []string{}
	for _, link := range links {
		all = append(all, link.Targets...)
	}
	return all
}

func targetsOf(links []store.Link, attribute string) []string {
	for _, link := range links {
		if link.Attribute == attribute {
			return link.Targets
		}
	}
	return nil
}
