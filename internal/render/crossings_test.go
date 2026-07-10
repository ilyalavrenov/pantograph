package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mkNode(qual, lane, kind string) *Node {
	return &Node{Flow: "x", Qual: qual, Lane: lane, Kind: kind, Pos: "f.go:1"}
}

func renderedCrossings(t *testing.T, f *Flow) int {
	t.Helper()

	res := renderOne(f, f.ID, testCompileCache, testShapes, "docs/flows")
	require.NoError(t, res.err, "fixture %q must compile", f.ID)

	edges := parseConnectionPaths([]byte(res.svg))
	require.NotEmpty(t, edges, "fixture %q produced no connection edges", f.ID)

	return countEdgeCrossings(edges)
}

func callbackDedupFixture() *Flow {
	const (
		store   = "store"
		wrk     = "worker"
		jobID   = "store.LookupByJobKey"
		batchID = "store.LookupByBatchKey"
	)

	return &Flow{
		ID: "callback-dedup",
		Nodes: []*Node{
			mkNode(jobID, store, "store"),
			mkNode(batchID, store, "store"),
			mkNode("api.handleUpdate", "api", "event"),
			mkNode("worker.reconcile", wrk, ""),
			mkNode("worker.matchRecords", wrk, ""),
			mkNode("worker.applyCallback", wrk, ""),
			mkNode("worker.alreadyApplied", wrk, KindDecision),
			mkNode("worker.matchedByKey", wrk, ""),
			mkNode("worker.matchedByJob", wrk, ""),
		},
		Edges: []Edge{
			{
				From: "worker.applyCallback",
				To:   "worker.alreadyApplied",
				Note: "write-side dedup before draining",
			},
			{From: "worker.alreadyApplied", To: "worker.matchedByKey", Cond: "real request id (async)"},
			{From: "worker.alreadyApplied", To: "worker.matchedByJob", Cond: "empty key (rediscover)"},
			{From: "worker.matchedByKey", To: batchID},
			{From: "worker.matchedByJob", To: jobID},
			{From: "worker.matchRecords", To: batchID, Cond: "primary key", Note: "stable key"},
			{From: "worker.matchRecords", To: jobID, Cond: "fallback: job key", Note: "diverges on retry"},
			{From: "worker.reconcile", To: "worker.matchRecords", Note: "pass 1: claim already-applied"},
			{From: "api.handleUpdate", To: "worker.applyCallback"},
		},
	}
}

func feedFanOutFixture() *Flow {
	const feed = "feed"

	n := func(name string) *Node { return mkNode("feed."+name, feed, "") }

	return &Flow{
		ID: "feed",
		Nodes: []*Node{
			n("Connect"), n("sessionLoop"), n("runOneSession"), n("runSession"),
			n("scanLoop"), n("resolveIDs"), n("startWatchdog"), n("establish"),
			n("runProvider"), n("subscribe"), n("subscribeByID"), n("stopClient"),
		},
		Edges: []Edge{
			{From: "feed.Connect", To: "feed.sessionLoop"},
			{From: "feed.sessionLoop", To: "feed.runOneSession"},
			{From: "feed.runOneSession", To: "feed.runSession"},
			{From: "feed.runSession", To: "feed.scanLoop"},
			{From: "feed.runSession", To: "feed.resolveIDs"},
			{From: "feed.runSession", To: "feed.startWatchdog"},
			{From: "feed.runSession", To: "feed.establish"},
			{From: "feed.resolveIDs", To: "feed.runProvider"},
			{From: "feed.startWatchdog", To: "feed.stopClient"},
			{From: "feed.establish", To: "feed.subscribe"},
			{From: "feed.establish", To: "feed.subscribeByID"},
		},
	}
}

func dailyBreakerFixture() *Flow {
	const (
		wrk = "worker"
		api = "api"
	)

	return &Flow{
		ID: "daily-breaker",
		Nodes: []*Node{
			mkNode("worker.passesFilters", wrk, KindDecision),
			mkNode("worker.checkLimitB", wrk, KindDecision),
			mkNode("worker.checkLimitA", wrk, KindDecision),
			mkNode("worker.tripLimit", wrk, ""),
			mkNode("worker.pollLoop", wrk, KindBackstop),
			mkNode("api.RejectAll", api, KindGateway),
		},
		Edges: []Edge{
			{
				From:    "worker.checkLimitA",
				To:      "worker.passesFilters",
				Cond:    "limit A breached",
				Note:    "passive: blocks new, never clears",
				Handoff: true,
			},
			{From: "worker.checkLimitB", To: "worker.tripLimit"},
			{From: "worker.pollLoop", To: "worker.checkLimitA"},
			{From: "worker.pollLoop", To: "worker.checkLimitB"},
			{
				From:    "worker.tripLimit",
				To:      "worker.passesFilters",
				Cond:    "switch set",
				Note:    "blocks the rest of the run",
				Handoff: true,
			},
			{From: "worker.tripLimit", To: "api.RejectAll"},
		},
	}
}

func TestRenderOneAvoidsEdgeCrossings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		fixture *Flow
		want    int
	}{
		{feedFanOutFixture(), 0},
		{callbackDedupFixture(), 1},
		{dailyBreakerFixture(), 0},
	}

	for _, tc := range cases {
		t.Run(tc.fixture.ID, func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tc.want, renderedCrossings(t, tc.fixture),
				"%s: rendered crossings != topological floor", tc.fixture.ID)
		})
	}
}
