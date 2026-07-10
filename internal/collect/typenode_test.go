package collect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nodeByQual(f *Flow, qual string) *Node {
	for _, n := range f.Nodes {
		if n.Qual == qual {
			return n
		}
	}

	return nil
}

func TestTypeNodeScannedAsNode(t *testing.T) {
	t.Parallel()

	f := collectFixture(t, "typenode")["tn"]
	require.NotNil(t, f, "flow tn must exist")

	source := nodeByQual(f, "typenode.Source")
	require.NotNil(t, source, "Source (single-spec type, doc on GenDecl) is a node")
	assert.Equal(t, "event", source.Kind, "explicit kind=event is honored on a type node")
	assert.Equal(t, "typenode", source.Lane, "the type node's lane is its package")

	sink := nodeByQual(f, "typenode.Sink")
	require.NotNil(t, sink, "Sink (grouped type, doc on TypeSpec) is a node")
	assert.Equal(t, "process", sink.Kind, "a blank kind on a type node defaults to process")

	done := nodeByQual(f, "typenode.Done")
	require.NotNil(t, done, "Done is a node")
	assert.Equal(t, "store", done.Kind, "explicit kind=store is honored on a type node")
}

func TestTypeNodeHandoffEdgesAndPerEndpointLabels(t *testing.T) {
	t.Parallel()

	f := collectFixture(t, "typenode")["tn"]
	require.NotNil(t, f)

	e, ok := edgeIn(f, "typenode.Source", "typenode.sinkFunc")
	require.True(t, ok, "Source→sinkFunc is a type→func handoff edge")
	assert.True(t, e.Handoff)
	assert.Equal(t, "feed in", e.Note, "the edge label sits on the type (-from) endpoint")

	_, ok = edgeIn(f, "typenode.Sink", "typenode.consume")
	assert.True(t, ok, "Sink→consume is a type→func handoff edge")

	_, ok = edgeIn(f, "typenode.consume", "typenode.Done")
	assert.True(t, ok, "consume→Done is a func→type handoff edge")

	toSource, ok := edgeIn(f, "typenode.fanSource", "typenode.Source")
	require.True(t, ok, "fanSource→Source (handoff fanA) must exist")
	assert.Equal(t, "to source", toSource.Note, "fanA's label binds to its own handoff token")

	toDone, ok := edgeIn(f, "typenode.fanSource", "typenode.Done")
	require.True(t, ok, "fanSource→Done (handoff fanB) must exist")
	assert.Equal(t, "to done", toDone.Note, "fanB's label binds to its own handoff token, not fanA's")
}
