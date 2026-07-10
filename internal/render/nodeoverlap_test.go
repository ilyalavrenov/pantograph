package render

import (
	"testing"

	"github.com/stretchr/testify/require"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestNodeLabelBoxesExtractsCenteredBoxIgnoresInline(t *testing.T) {
	t.Parallel()

	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	svg := []byte(`<text x="360.000000" y="305.500000" fill="#000410" ` +
		`class="text-mono-bold fill-N1" style="text-anchor:middle;font-size:16px">loadSession</text>`)

	boxes := nodeLabelBoxes(svg, ruler)
	require.Len(t, boxes, 1, "node-label boxes")

	b := boxes[0]

	require.InDelta(t, 360.0, b.x+b.w/2, 1.0, "box center x")

	require.True(t, b.y < 305.5 && 305.5 < b.y+b.h,
		"baseline 305.5 must fall inside box Y[%.1f,%.1f]", b.y, b.y+b.h)

	require.Positive(t, b.w, "degenerate box")
	require.Positive(t, b.h, "degenerate box")

	inline := []byte(`<g transform="translate(71.000000 474.000000)" class="light-code">` +
		`<text class="text-mono" x="0" y="1.000000em">limit A breached</text></g>`)

	require.Empty(t, nodeLabelBoxes(inline, ruler),
		"inline label must not be returned as a node box")
}

func TestInlineOverlapsNode(t *testing.T) {
	t.Parallel()

	abutting := labelBox{x: 71, y: 474, w: 482 - 71, h: 516 - 474}
	node := labelBox{x: 376, y: 516, w: 914 - 376, h: 572 - 516}

	require.True(t, abutting.overlapsNode([]labelBox{node}),
		"an inline chip abutting a node (x-overlap, y-touch) must fire")

	separated := labelBox{x: 200, y: 516, w: 120, h: 30}
	farNode := labelBox{x: 376, y: 516, w: 100, h: 30}

	require.False(t, separated.overlapsNode([]labelBox{farNode}),
		"a clearly-separated inline label must NOT fire on a nearby node")
}
