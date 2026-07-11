package render

import (
	"fmt"
	"testing"
)

func smallFlow(id string) *Flow {
	return &Flow{ID: id, Nodes: []*Node{
		{Flow: id, Qual: "api.Validate", Kind: KindDecision, Lane: "api", Pos: "f.go:1"},
		{Flow: id, Qual: "api.Enqueue", Lane: "api", Pos: "f.go:2"},
		{Flow: id, Qual: "worker.Dequeue", Lane: "worker", Pos: "f.go:3"},
		{Flow: id, Qual: "worker.Store", Kind: KindStore, Lane: "worker", Pos: "f.go:4"},
	}, Edges: []Edge{
		{From: "api.Validate", To: "api.Enqueue", Cond: "valid", Note: "accepted"},
		{From: "api.Enqueue", To: "worker.Dequeue", Handoff: true, Note: "jobs"},
		{From: "worker.Dequeue", To: "worker.Store"},
	}}
}

func bigFlow(id string) *Flow {
	const lanes, perLane = 4, 8

	laneNames := []string{"api", "worker", "store", "auth"}
	nodes := make([]*Node, 0, lanes*perLane)
	qual := func(l, i int) string { return fmt.Sprintf("%s.Step%02d", laneNames[l], i) }

	for l := range lanes {
		for i := range perLane {
			kind := ""
			switch i % 4 {
			case 1:
				kind = KindDecision
			case 3:
				kind = KindStore
			}

			nodes = append(nodes, &Node{
				Flow: id, Qual: qual(l, i), Kind: kind,
				Lane: laneNames[l], Pos: fmt.Sprintf("f.go:%d", l*perLane+i),
			})
		}
	}

	var edges []Edge
	for l := range lanes {
		for i := range perLane - 1 {
			edges = append(edges, Edge{From: qual(l, i), To: qual(l, i+1), Cond: "ok"})
		}

		if l < lanes-1 {
			edges = append(edges,
				Edge{From: qual(l, perLane-1), To: qual(l+1, 0), Handoff: true, Note: "handoff"},
				Edge{From: qual(l, perLane/2), To: qual(l+1, perLane/2), Note: "fanout"},
			)
		}
	}

	return &Flow{ID: id, Nodes: nodes, Edges: edges}
}

func BenchmarkRenderBatch(b *testing.B) {
	shapes := []struct {
		name string
		make func(string) *Flow
	}{
		{"small", smallFlow},
		{"big", bigFlow},
	}

	for _, shape := range shapes {
		for _, n := range []int{1, 10, 25} {
			b.Run(fmt.Sprintf("%s/n=%d", shape.name, n), func(b *testing.B) {
				flows := make(map[string]*Flow, n)
				for i := range n {
					id := fmt.Sprintf("flow%02d", i)
					flows[id] = shape.make(id)
				}

				b.ReportMetric(float64(n), "diagrams/op")
				b.ResetTimer()

				for b.Loop() {
					if _, err := Render(flows, testShapes, "docs/flows"); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
