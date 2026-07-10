package collect

import (
	"sort"
	"strings"
	"testing"

	"github.com/ilyalavrenov/pantograph/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDomainTableDetectsConflicts(t *testing.T) {
	t.Parallel()

	t.Run("flow in two domains", func(t *testing.T) {
		t.Parallel()

		_, errs := buildDomainTable([]config.DomainDecl{
			{Domain: "a", Flows: []string{"shared"}},
			{Domain: "b", Flows: []string{"shared"}},
		})
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "two domains")
		assert.Contains(t, errs[0], "shared")
	})

	t.Run("domain declared twice", func(t *testing.T) {
		t.Parallel()

		_, errs := buildDomainTable([]config.DomainDecl{
			{Domain: "a", Flows: []string{"x"}},
			{Domain: "a", Flows: []string{"y"}},
		})
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0], "twice")
	})

	t.Run("clean table", func(t *testing.T) {
		t.Parallel()

		tbl, errs := buildDomainTable([]config.DomainDecl{
			{Domain: "a", Flows: []string{"x", "y"}, Note: "blurb"},
		})
		require.Empty(t, errs)
		d, ok := tbl.flowToDomain["x"]
		assert.True(t, ok)
		assert.Equal(t, "a", d)
		assert.Equal(t, "blurb", tbl.note["a"])
	})
}

func TestValidateDomains(t *testing.T) {
	t.Parallel()

	flows := map[string]*Flow{
		"mapped":   {ID: "mapped", Nodes: []*Node{{Qual: "p.A", Pos: "a.go:1"}}},
		"unmapped": {ID: "unmapped", Nodes: []*Node{{Qual: "p.B", Pos: "b.go:9"}}},
	}

	tbl := &domainTable{
		flowToDomain: map[string]string{"mapped": "d", "ghost": "d"},
		note:         map[string]string{},
	}

	errs := validateDomains(flows, tbl)

	joined := strings.Join(errs, "\n")
	assert.Contains(t, joined, "unmapped", "an unclaimed flow must error")
	assert.Contains(t, joined, "b.go:9", "the error points at the flow's source")
	assert.Contains(t, joined, "ghost", "a dangling member must error")
}

func TestFuseDomains(t *testing.T) {
	t.Parallel()

	t.Run("unions and dedups", func(t *testing.T) {
		t.Parallel()

		flows := map[string]*Flow{
			"f": {ID: "f", Nodes: []*Node{
				{Flow: "f", Qual: "p.A", Lane: "p", Pos: "a.go:1"},
				{Flow: "f", Qual: "p.SHARED", Lane: "p", Pos: "s.go:1"},
			}, Edges: []Edge{{From: "p.A", To: "p.SHARED"}}},
			"g": {ID: "g", Nodes: []*Node{
				{Flow: "g", Qual: "p.SHARED", Lane: "p", Pos: "s.go:1"},
				{Flow: "g", Qual: "p.B", Lane: "p", Pos: "b.go:1"},
			}, Edges: []Edge{{From: "p.SHARED", To: "p.B"}}},
		}

		tbl := &domainTable{
			flowToDomain: map[string]string{"f": "d", "g": "d"},
			note:         map[string]string{"d": "the d domain"},
		}

		domains, errs := fuseDomains(flows, tbl)
		require.Empty(t, errs)
		require.Contains(t, domains, "d")

		d := domains["d"]
		assert.Equal(t, []string{"f", "g"}, d.Members)
		assert.Equal(t, "the d domain", d.Note)
		assert.Equal(t, []string{"p.A", "p.B", "p.SHARED"}, sortedQuals(d.Nodes), "SHARED deduped to one node")
		assert.Len(t, d.Edges, 2, "both edges survive the union")
	})

	t.Run("conflicting edge labels error", func(t *testing.T) {
		t.Parallel()

		flows := map[string]*Flow{
			"f": {ID: "f", Nodes: []*Node{
				{Flow: "f", Qual: "p.A", Lane: "p", Pos: "a.go:1"},
				{Flow: "f", Qual: "p.B", Lane: "p", Pos: "b.go:1"},
			}, Edges: []Edge{{From: "p.A", To: "p.B", Cond: "won"}}},
			"g": {ID: "g", Nodes: []*Node{
				{Flow: "g", Qual: "p.A", Lane: "p", Pos: "a.go:1"},
				{Flow: "g", Qual: "p.B", Lane: "p", Pos: "b.go:1"},
			}, Edges: []Edge{{From: "p.A", To: "p.B", Cond: "lost"}}},
		}

		tbl := &domainTable{
			flowToDomain: map[string]string{"f": "d", "g": "d"},
			note:         map[string]string{},
		}

		_, errs := fuseDomains(flows, tbl)
		require.NotEmpty(t, errs)
		assert.Contains(t, strings.Join(errs, "\n"), "conflicting cond")
	})
}

func sortedQuals(nodes []*Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Qual
	}

	sort.Strings(out)

	return out
}
