package collect

import (
	"fmt"
	"sort"
	"strings"
)

type FuncInfo struct {
	Qual      string
	PkgPath   string
	Pos       string
	Annotated bool
}

func ListReport(flows map[string]*Flow) []string {
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	var lines []string

	for _, id := range ids {
		f := flows[id]

		lines = append(lines, fmt.Sprintf("%s (%s)", f.ID, pluralSteps(len(f.Nodes))))

		for _, n := range f.Nodes {
			lines = append(lines, fmt.Sprintf("  %s — %s (%s)", FuncLabel(n.Qual), n.Qual, n.Pos))
		}
	}

	return lines
}

func pluralSteps(n int) string {
	if n == 1 {
		return "1 step"
	}

	return fmt.Sprintf("%d steps", n)
}

func CoverageReport(inv []FuncInfo, filter string) ([]string, bool) {
	var lines []string

	filterMatched := filter == ""

	for _, fi := range inv {
		if !matchesPkgFilter(fi.PkgPath, filter) {
			continue
		}

		filterMatched = true

		if fi.Annotated {
			continue
		}

		lines = append(lines, fmt.Sprintf("%s (%s)", fi.Qual, fi.Pos))
	}

	sort.Strings(lines)

	return lines, filterMatched
}

func matchesPkgFilter(pkgPath, filter string) bool {
	if filter == "" {
		return true
	}

	filter = strings.TrimSuffix(filter, "/")

	return pkgPath == filter || strings.HasPrefix(pkgPath, filter+"/")
}

func FindOrphanNodes(flows map[string]*Flow) []string {
	var warnings []string

	for _, f := range flows {
		if len(f.Nodes) < 2 {
			continue
		}

		touched := map[string]bool{}

		for _, e := range f.Edges {
			touched[e.From] = true
			touched[e.To] = true
		}

		for _, n := range f.Nodes {
			if touched[n.Qual] {
				continue
			}

			warnings = append(warnings, fmt.Sprintf(
				"%s: flow %q: node %q has no inbound or outbound edge (orphan — missing handoff or a typo'd flow-id?)",
				n.Pos, f.ID, FuncLabel(n.Qual)))
		}
	}

	sort.Strings(warnings)

	return warnings
}
