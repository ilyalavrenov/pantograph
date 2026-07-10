package collect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixturePattern(name string) string {
	return "./internal/collect/testdata/" + name + "/..."
}

func collectFixture(t *testing.T, name string) map[string]*Flow {
	t.Helper()

	flows, _, _, err := Collect([]string{fixturePattern(name)})
	require.NoError(t, err)

	return flows
}

func edgeIn(f *Flow, from, to string) (Edge, bool) {
	for _, e := range f.Edges {
		if e.From == from && e.To == to {
			return e, true
		}
	}

	return Edge{}, false
}

func nodeQuals(f *Flow) []string {
	qs := make([]string, 0, len(f.Nodes))
	for _, n := range f.Nodes {
		qs = append(qs, n.Qual)
	}

	return qs
}

func TestDeriveDirectAndUntaggedAndCrossFlow(t *testing.T) {
	t.Parallel()

	flows := collectFixture(t, "derive")

	f := flows["f"]
	require.NotNil(t, f, "flow f must exist")

	_, ok := edgeIn(f, "derive.funcA", "derive.funcB")
	assert.True(t, ok, "funcA→funcB is a derived edge (direct same-flow call)")

	_, okD := edgeIn(f, "derive.funcA", "derive.funcD")
	assert.False(t, okD, "no edge to an UNtagged callee (funcD)")
	_, okE := edgeIn(f, "derive.funcA", "derive.funcE")
	assert.False(t, okE, "no cross-flow edge (funcE is in flow g)")

	for _, e := range f.Edges {
		assert.NotEqual(t, "derive.funcC", e.From, "funcC has no outbound edge (only an untagged callee)")
		assert.NotEqual(t, "derive.funcC", e.To, "funcC has no inbound edge")
	}

	assert.NotContains(t, nodeQuals(f), "derive.funcE", "funcE is not a node in flow f")

	g := flows["g"]
	require.NotNil(t, g, "flow g must exist")
	assert.Equal(t, []string{"derive.funcE"}, nodeQuals(g), "flow g holds only funcE")
	assert.Empty(t, g.Edges, "flow g has no edges")

	_, okBridge := edgeIn(f, "derive.caller", "derive.impl.M")
	assert.True(t, okBridge, "caller→impl.M derives via the interface name-bridge (i.M() resolves to iface.M)")
}

func TestHandoffPairingDirectionAndCondLeak(t *testing.T) {
	t.Parallel()

	f := collectFixture(t, "handoff")["f"]
	require.NotNil(t, f)

	e, ok := edgeIn(f, "handoff.simpleFrom", "handoff.simpleTo")
	require.True(t, ok, "the simple handoff draws simpleFrom→simpleTo (no call between them)")
	assert.True(t, e.Handoff, "a paired edge is marked handoff")

	_, rev := edgeIn(f, "handoff.simpleTo", "handoff.simpleFrom")
	assert.False(t, rev, "handoff direction is from -from to -to only, no reverse edge")

	out, ok := edgeIn(f, "handoff.dualNode", "handoff.dstB")
	require.True(t, ok, "dualNode→dstB handoff edge must exist")
	assert.Equal(t, "why", out.Cond, "cond labels the OUTGOING handoff edge (dualNode is the -from)")
	assert.True(t, out.Handoff)

	in, ok := edgeIn(f, "handoff.srcA", "handoff.dualNode")
	require.True(t, ok, "srcA→dualNode handoff edge must exist")
	assert.Empty(t, in.Cond, "cond must NOT leak onto the INCOMING handoff edge (dualNode is the -to)")
	assert.True(t, in.Handoff)
}

func TestHandoffOneSidedErrors(t *testing.T) {
	t.Parallel()

	_, _, _, err := Collect([]string{fixturePattern("handoff_onesided")})
	require.Error(t, err, "a one-sided handoff must fail the build")
	assert.Contains(t, err.Error(), "orphan", "the error names the unpaired handoff")
	assert.Contains(t, err.Error(), "only one endpoint")
}

func TestCallSiteLabelLandsOnDerivedEdge(t *testing.T) {
	t.Parallel()

	f := collectFixture(t, "callsite")["f"]
	require.NotNil(t, f)

	e, ok := edgeIn(f, "callsite.entry", "callsite.helper")
	require.True(t, ok, "entry→helper is a derived edge")
	assert.Equal(t, "async via goroutine", e.Note, "the call-site note labels the derived edge")
	assert.False(t, e.Handoff, "a call-site-labeled edge is a derived (called) edge, not a handoff")
}

func TestGoroutineEdgeDetection(t *testing.T) {
	t.Parallel()

	f := collectFixture(t, "goroutine")["g"]
	require.NotNil(t, f)

	async, ok := edgeIn(f, "goroutine.dispatcher", "goroutine.loop")
	require.True(t, ok, "dispatcher→loop is a derived edge")
	assert.True(t, async.Goroutine, "a `go`-dispatched call site marks its edge goroutine")

	seq, ok := edgeIn(f, "goroutine.dispatcher", "goroutine.sync")
	require.True(t, ok, "dispatcher→sync is a derived edge")
	assert.False(t, seq.Goroutine, "a sequential call site does not mark its edge goroutine")

	nested, ok := edgeIn(f, "goroutine.wrapped", "goroutine.inner")
	require.True(t, ok, "wrapped→inner is a derived edge")
	assert.True(t, nested.Goroutine, "a call inside a `go func(){…}()` literal is async")

	merged, ok := edgeIn(f, "goroutine.orMerge", "goroutine.dup")
	require.True(t, ok, "orMerge→dup is a single derived edge")
	assert.True(t, merged.Goroutine, "the goroutine flag is OR-ed across call sites (an async path exists)")

	dispatched, ok := edgeIn(f, "goroutine.argDispatch", "goroutine.consume")
	require.True(t, ok, "argDispatch→consume is a derived edge")
	assert.True(t, dispatched.Goroutine, "the dispatched call (go consume(...)) is async")

	arg, ok := edgeIn(f, "goroutine.argDispatch", "goroutine.produce")
	require.True(t, ok, "argDispatch→produce is a derived edge")
	assert.False(t, arg.Goroutine, "a call in ARGUMENT position of a `go` dispatch is synchronous, not async")
}
