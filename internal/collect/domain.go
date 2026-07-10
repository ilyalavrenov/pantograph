package collect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ilyalavrenov/pantograph/internal/config"
)

type domainTable struct {
	flowToDomain map[string]string
	note         map[string]string
}

func buildDomainTable(decls []config.DomainDecl) (*domainTable, []string) {
	t := &domainTable{
		flowToDomain: map[string]string{},
		note:         map[string]string{},
	}

	var errs []string

	for _, d := range decls {
		if _, dup := t.note[d.Domain]; dup {
			errs = append(errs, fmt.Sprintf("domain %q declared twice", d.Domain))
		} else {
			t.note[d.Domain] = d.Note
		}

		for _, fid := range d.Flows {
			if owner, taken := t.flowToDomain[fid]; taken {
				errs = append(errs, fmt.Sprintf(
					"flow %q is in two domains (%q and %q) — a flow belongs to exactly one",
					fid, owner, d.Domain,
				))

				continue
			}

			t.flowToDomain[fid] = d.Domain
		}
	}

	sort.Strings(errs)

	return t, errs
}

func validateDomains(flows map[string]*Flow, t *domainTable) []string {
	var errs []string

	for id, f := range flows {
		if _, ok := t.flowToDomain[id]; ok {
			continue
		}

		pos := "?"
		if len(f.Nodes) > 0 {
			pos = f.Nodes[0].Pos
		}

		errs = append(errs, fmt.Sprintf("%s: flow %q has no domain — add it to a domains: entry in pantograph.yaml", pos, id))
	}

	for fid, dom := range t.flowToDomain {
		if _, ok := flows[fid]; !ok {
			errs = append(errs, fmt.Sprintf("domain %q: flows= names %q, which is not a known flow (typo, or the flow was deleted)", dom, fid))
		}
	}

	sort.Strings(errs)

	return errs
}

func FuseFlows(flows map[string]*Flow, decls []config.DomainDecl) (map[string]*Flow, error) {
	t, errs := buildDomainTable(decls)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%d domain declaration error(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}

	if errs := validateDomains(flows, t); len(errs) > 0 {
		return nil, fmt.Errorf("%d domain coverage error(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}

	domains, errs := fuseDomains(flows, t)
	if len(errs) > 0 {
		return nil, fmt.Errorf("%d domain fusion error(s):\n%s", len(errs), strings.Join(errs, "\n"))
	}

	return domains, nil
}

func fuseDomains(flows map[string]*Flow, t *domainTable) (map[string]*Flow, []string) {
	members := map[string][]string{}
	for id := range flows {
		d := t.flowToDomain[id]
		members[d] = append(members[d], id)
	}

	for d := range members {
		sort.Strings(members[d])
	}

	fused := make(map[string]*Flow, len(members))

	var errs []string

	for d, ids := range members {
		f, errsD := fuseDomain(d, ids, flows, t)
		fused[d] = f
		errs = append(errs, errsD...)
	}

	sort.Strings(errs)

	return fused, errs
}

func fuseDomain(domain string, ids []string, flows map[string]*Flow, t *domainTable) (*Flow, []string) {
	f := &Flow{ID: domain, Members: append([]string(nil), ids...), Note: t.note[domain]}

	seenNode := map[string]bool{}

	var errs []string

	for _, id := range ids {
		for _, n := range flows[id].Nodes {
			if seenNode[n.Qual] {
				continue
			}

			seenNode[n.Qual] = true

			c := *n
			f.Nodes = append(f.Nodes, &c)
		}
	}

	edgeAt := map[string]int{}

	addEdge := func(e Edge) {
		key := e.From + "\x00" + e.To
		if i, ok := edgeAt[key]; ok {
			if c := mergeEdge(&f.Edges[i], e); c != "" {
				errs = append(errs, fmt.Sprintf("domain %q: %s on edge %s -> %s", domain, c, FuncLabel(e.From), FuncLabel(e.To)))
			}

			return
		}

		edgeAt[key] = len(f.Edges)
		f.Edges = append(f.Edges, e)
	}

	for _, id := range ids {
		for _, e := range flows[id].Edges {
			addEdge(e)
		}
	}

	sortEdges(f.Edges)

	return f, errs
}

func mergeEdge(dst *Edge, src Edge) string {
	if c := mergeLabel(&dst.Cond, src.Cond, "cond"); c != "" {
		return c
	}

	if c := mergeLabel(&dst.Note, src.Note, "note"); c != "" {
		return c
	}

	dst.Handoff = dst.Handoff || src.Handoff

	return ""
}

func mergeLabel(dst *string, src, name string) string {
	switch {
	case src == "" || src == *dst:
		return ""
	case *dst == "":
		*dst = src

		return ""
	default:
		return fmt.Sprintf("conflicting %s= labels %q vs %q", name, *dst, src)
	}
}
