package collect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectBuildsCoverageInventory(t *testing.T) {
	t.Parallel()

	_, inv, _, err := Collect([]string{fixturePattern("collectbasic")})
	require.NoError(t, err)
	require.NotEmpty(t, inv, "the fixture has exported funcs, so the inventory must be populated")

	var sawAnnotated, sawUnannotated bool

	for _, fi := range inv {
		assert.True(t, strings.HasPrefix(fi.PkgPath, "internal/collect/testdata/collectbasic"),
			"inventory pkgPath %q must be module-relative", fi.PkgPath)

		if fi.Annotated {
			sawAnnotated = true
		} else {
			sawUnannotated = true
		}
	}

	assert.True(t, sawAnnotated, "the fixture has //pantograph: annotated exported funcs (Entry/Step)")
	assert.True(t, sawUnannotated, "the fixture also has un-annotated exported funcs (Unannotated)")

	rep, matched := CoverageReport(inv, "internal/collect/testdata/collectbasic")
	require.NotEmpty(t, rep)
	assert.True(t, matched, "the fixture is a real scanned package")
	assert.IsIncreasing(t, rep, "coverage output must be sorted")
}

func TestListReport(t *testing.T) {
	t.Parallel()

	flows := map[string]*Flow{
		"zeta": {ID: "zeta", Nodes: []*Node{
			{Flow: "zeta", Qual: "p.Z1", Pos: "z.go:1"},
		}},
		"alpha": {ID: "alpha", Nodes: []*Node{
			{Flow: "alpha", Qual: "p.A1", Pos: "a.go:1"},
			{Flow: "alpha", Qual: "p.A2", Pos: "a.go:2"},
		}},
	}

	got := ListReport(flows)

	assert.Equal(t, []string{
		"alpha (2 steps)",
		"  A1 — p.A1 (a.go:1)",
		"  A2 — p.A2 (a.go:2)",
		"zeta (1 step)",
		"  Z1 — p.Z1 (z.go:1)",
	}, got)
}

func TestCoverageReport(t *testing.T) {
	t.Parallel()

	inv := []FuncInfo{
		{Qual: "api.Zztop", PkgPath: "pkg/api", Pos: "pkg/api/z.go:9", Annotated: false},
		{Qual: "api.Apply", PkgPath: "pkg/api", Pos: "pkg/api/a.go:3", Annotated: false},
		{Qual: "api.MarkDone", PkgPath: "pkg/api", Pos: "pkg/api/b.go:5", Annotated: true},
		{Qual: "worker.Fold", PkgPath: "pkg/worker", Pos: "pkg/worker/f.go:1", Annotated: false},
	}

	all, allMatched := CoverageReport(inv, "")
	assert.Equal(t, []string{
		"api.Apply (pkg/api/a.go:3)",
		"api.Zztop (pkg/api/z.go:9)",
		"worker.Fold (pkg/worker/f.go:1)",
	}, all, "annotated MarkDone is excluded; rest sorted")
	assert.True(t, allMatched, "the empty (all-packages) filter always matches")

	only, onlyMatched := CoverageReport(inv, "pkg/api")
	assert.Equal(t, []string{
		"api.Apply (pkg/api/a.go:3)",
		"api.Zztop (pkg/api/z.go:9)",
	}, only)
	assert.True(t, onlyMatched, "pkg/api is present in the inventory")

	none, noneMatched := CoverageReport(inv, "pkg/aip")
	assert.Empty(t, none)
	assert.False(t, noneMatched, "a typo'd filter matches nothing and must signal it")

	subtree := []FuncInfo{
		{Qual: "api.Apply", PkgPath: "pkg/api", Pos: "pkg/api/a.go:3"},
		{Qual: "invariants.Check", PkgPath: "pkg/api/invariants", Pos: "pkg/api/invariants/c.go:1"},
		{Qual: "worker.Fold", PkgPath: "pkg/worker", Pos: "pkg/worker/f.go:1"},
	}

	got, gotMatched := CoverageReport(subtree, "pkg/api")
	assert.True(t, gotMatched)
	assert.Equal(t, []string{
		"api.Apply (pkg/api/a.go:3)",
		"invariants.Check (pkg/api/invariants/c.go:1)",
	}, got, "prefix match pulls in the subtree but not a sibling package")
}

func TestFindOrphanNodes(t *testing.T) {
	t.Parallel()

	t.Run("orphan in multi-node flow is flagged", func(t *testing.T) {
		t.Parallel()

		flows := map[string]*Flow{
			"f": {ID: "f", Nodes: []*Node{
				{Flow: "f", Qual: "p.A", Pos: "f.go:1"},
				{Flow: "f", Qual: "p.B", Pos: "f.go:2"},
				{Flow: "f", Qual: "p.Island", Pos: "f.go:9"},
			}, Edges: []Edge{{From: "p.A", To: "p.B"}}},
		}

		got := FindOrphanNodes(flows)
		assert.Len(t, got, 1)
		assert.Contains(t, got[0], "f.go:9")
		assert.Contains(t, got[0], "Island")
		assert.Contains(t, got[0], `flow "f"`)
	})

	t.Run("single-node flow does not warn", func(t *testing.T) {
		t.Parallel()

		flows := map[string]*Flow{
			"solo": {ID: "solo", Nodes: []*Node{
				{Flow: "solo", Qual: "p.Only", Pos: "s.go:1"},
			}},
		}

		assert.Empty(t, FindOrphanNodes(flows), "a single-node flow is legitimately edge-less")
	})

	t.Run("multiple orphans report sorted by position", func(t *testing.T) {
		t.Parallel()

		flows := map[string]*Flow{
			"f": {ID: "f", Nodes: []*Node{
				{Flow: "f", Qual: "p.A", Pos: "f.go:1"},
				{Flow: "f", Qual: "p.B", Pos: "f.go:2"},
				{Flow: "f", Qual: "p.Y", Pos: "f.go:30"},
				{Flow: "f", Qual: "p.X", Pos: "f.go:20"},
			}, Edges: []Edge{{From: "p.A", To: "p.B"}}},
		}

		got := FindOrphanNodes(flows)
		assert.Len(t, got, 2)
		assert.Contains(t, got[0], "f.go:20")
		assert.Contains(t, got[1], "f.go:30")
	})
}
