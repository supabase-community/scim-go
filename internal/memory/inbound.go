package memory

import "github.com/supabase-community/scim-go/pkg/store"

type inbound map[string]map[edge]struct{}

func (in inbound) add(target string, e edge) {
	if in[target] == nil {
		in[target] = map[edge]struct{}{}
	}
	in[target][e] = struct{}{}
}

func (in inbound) remove(target string, e edge) {
	delete(in[target], e)
}

func (in inbound) sources(target, attribute string) []string {
	sources := []string{}
	for e := range in[target] {
		if e.attribute == attribute {
			sources = append(sources, e.source)
		}
	}
	return sources
}

func (in inbound) drop(target string) []edge {
	edges := make([]edge, 0, len(in[target]))
	for e := range in[target] {
		edges = append(edges, e)
	}
	delete(in, target)
	return edges
}

func (in inbound) relink(source string, before, after []store.Link) {
	for _, link := range before {
		for _, target := range link.Targets {
			in.remove(target, edge{source, link.Attribute})
		}
	}
	for _, link := range after {
		for _, target := range link.Targets {
			in.add(target, edge{source, link.Attribute})
		}
	}
}
