package render

import (
	"sort"

	"github.com/ilyalavrenov/pantograph/internal/collect"
)

func reorderCandidates(nodes []*collect.Node, edges []collect.Edge) [][]*collect.Node {
	if len(edges) == 0 || len(nodes) < 3 {
		return nil
	}

	reversed := make([]*collect.Node, len(nodes))
	for i, n := range nodes {
		reversed[len(nodes)-1-i] = n
	}

	return [][]*collect.Node{
		barycenterOrder(nodes, edges),
		barycenterOrder(reversed, edges),
	}
}

func barycenterOrder(nodes []*collect.Node, edges []collect.Edge) []*collect.Node {
	idx := make(map[string]int, len(nodes))
	for i, n := range nodes {
		idx[n.Qual] = i
	}

	succ, pred := adjacency(nodes, edges, idx)
	rank := longestPathRanks(nodes, pred)
	ranks := groupByRank(nodes, rank)

	maxRank := 0
	for _, r := range rank {
		if r > maxRank {
			maxRank = r
		}
	}

	pos := make([]int, len(nodes))
	for _, layer := range ranks {
		for p, i := range layer {
			pos[i] = p
		}
	}

	for r := 1; r <= maxRank; r++ {
		reorderByBarycenter(ranks[r], pred, pos)
	}

	for r := maxRank - 1; r >= 0; r-- {
		reorderByBarycenter(ranks[r], succ, pos)
	}

	ordered := make([]*collect.Node, 0, len(nodes))
	for r := 0; r <= maxRank; r++ {
		layer := ranks[r]
		sort.SliceStable(layer, func(a, b int) bool { return pos[layer[a]] < pos[layer[b]] })

		for _, i := range layer {
			ordered = append(ordered, nodes[i])
		}
	}

	return ordered
}

func adjacency(nodes []*collect.Node, edges []collect.Edge, idx map[string]int) ([][]int, [][]int) {
	succ := make([][]int, len(nodes))
	pred := make([][]int, len(nodes))

	for _, e := range edges {
		s, sok := idx[e.From]
		d, dok := idx[e.To]
		if !sok || !dok || s == d {
			continue
		}

		succ[s] = append(succ[s], d)
		pred[d] = append(pred[d], s)
	}

	return succ, pred
}

func longestPathRanks(nodes []*collect.Node, pred [][]int) []int {
	rank := make([]int, len(nodes))
	computed := make([]bool, len(nodes))

	var rankOf func(i int) int
	rankOf = func(i int) int {
		if computed[i] {
			return rank[i]
		}

		best := 0
		for _, p := range pred[i] {
			if p >= i {
				continue
			}

			if r := rankOf(p) + 1; r > best {
				best = r
			}
		}

		rank[i] = best
		computed[i] = true

		return best
	}

	for i := range nodes {
		rankOf(i)
	}

	return rank
}

func groupByRank(nodes []*collect.Node, rank []int) [][]int {
	maxRank := 0
	for _, r := range rank {
		if r > maxRank {
			maxRank = r
		}
	}

	ranks := make([][]int, maxRank+1)
	for i := range nodes {
		ranks[rank[i]] = append(ranks[rank[i]], i)
	}

	return ranks
}

func reorderByBarycenter(layer []int, adj [][]int, pos []int) {
	if len(layer) < 2 {
		return
	}

	key := make(map[int]float64, len(layer))
	for _, i := range layer {
		neighbours := adj[i]
		if len(neighbours) == 0 {
			key[i] = float64(pos[i])

			continue
		}

		sum := 0
		for _, j := range neighbours {
			sum += pos[j]
		}

		key[i] = float64(sum) / float64(len(neighbours))
	}

	sort.SliceStable(layer, func(a, b int) bool {
		return key[layer[a]] < key[layer[b]]
	})

	for p, i := range layer {
		pos[i] = p
	}
}
