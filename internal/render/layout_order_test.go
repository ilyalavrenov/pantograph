package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func ranksFor(nodes []*Node, edges []Edge) map[string]int {
	idx := make(map[string]int, len(nodes))
	for i, n := range nodes {
		idx[n.Qual] = i
	}

	_, pred := adjacency(nodes, edges, idx)
	rank := longestPathRanks(nodes, pred)

	out := make(map[string]int, len(nodes))
	for i, n := range nodes {
		out[n.Qual] = rank[i]
	}

	return out
}

func TestLongestPathRanksUsesLongestNotFirstPath(t *testing.T) {
	t.Parallel()

	nodes := []*Node{{Qual: "a"}, {Qual: "b"}, {Qual: "c"}, {Qual: "d"}, {Qual: "e"}}
	edges := []Edge{
		{From: "a", To: "d"},
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "d"},
		{From: "d", To: "e"},
	}

	r := ranksFor(nodes, edges)

	assert.Equal(t, 0, r["a"], "source")
	assert.Equal(t, 1, r["b"])
	assert.Equal(t, 2, r["c"])
	assert.Equal(t, 3, r["d"], "longest path a->b->c->d, not the short a->d")
	assert.Equal(t, 4, r["e"], "downstream of d must follow d's longest-path rank")
}

func TestLongestPathRanksTerminatesOnCycle(t *testing.T) {
	t.Parallel()

	nodes := []*Node{{Qual: "a"}, {Qual: "b"}, {Qual: "c"}}
	edges := []Edge{
		{From: "a", To: "b"},
		{From: "b", To: "c"},
		{From: "c", To: "a"},
	}

	r := ranksFor(nodes, edges)

	assert.Equal(t, 0, r["a"])
	assert.Equal(t, 1, r["b"])
	assert.Equal(t, 2, r["c"])
}
