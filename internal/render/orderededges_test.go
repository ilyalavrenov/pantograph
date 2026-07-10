package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func edgeTargets(edges []Edge) []string {
	out := make([]string, len(edges))
	for i, e := range edges {
		out[i] = e.To
	}

	return out
}

func TestOrderedEdgesSortsFanOutByTargetColumn(t *testing.T) {
	t.Parallel()

	f := &Flow{
		ID: "x",
		Nodes: []*Node{
			{Flow: "x", Qual: "z.a", Lane: "z", Pos: "f.go:1"},
			{Flow: "x", Qual: "a.la", Lane: "a", Pos: "f.go:2"},
			{Flow: "x", Qual: "m.ma", Lane: "m", Pos: "f.go:3"},
			{Flow: "x", Qual: "z.za", Lane: "z", Pos: "f.go:4"},
		},
		Edges: []Edge{
			{From: "z.a", To: "z.za"},
			{From: "z.a", To: "a.la"},
			{From: "z.a", To: "m.ma"},
		},
	}

	got := edgeTargets(orderedEdges(f))
	want := []string{"a.la", "m.ma", "z.za"}

	require.Equal(t, want, got, "fan-out target order")
}
