package render

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestEdgeBleedsThrough(t *testing.T) {
	t.Parallel()

	box := labelBox{x: 100, y: 200, w: 80, h: 24, text: "lbl"}

	seg := func(x float64) segment { return segment{x: x, yTop: 190, yBot: 230} }

	cases := []struct {
		name string
		segs []segment
		want bool
	}{
		{"own edge within tol does not bleed", []segment{seg(140 + ownEdgeTol - 1)}, false},
		{"off-center foreign edge bleeds", []segment{seg(140 + ownEdgeTol + 5)}, true},
		{"foreign edge grazing chip's left flank bleeds", []segment{seg(box.x - chipTextInset + 1)}, true},
		{"edge outside chip span ignored", []segment{seg(box.x - chipTextInset - 5)}, false},
		{"edge not overlapping band ignored", []segment{{x: 140 + ownEdgeTol + 5, yTop: 100, yBot: 150}}, false},
		{"no segments never bleeds", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, box.edgeBleedsThrough(tc.segs))
		})
	}
}

func TestCollidingEdgeKeysDetectsSingleLabelBleed(t *testing.T) {
	t.Parallel()

	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	const label = "daily cache"

	box := inlineLabelSVGBox(20, 100, label, ruler)
	foreignX := box.x + box.w/2 + ownEdgeTol + 30

	svg := []byte(syntheticBleedSVG(20, 100, label, foreignX))

	f := &Flow{
		ID:    "bleed-single",
		Edges: []Edge{{From: "lane.src", To: "lane.dst", Note: label}},
	}

	got := collidingEdgeKeys(svg, f, ruler)
	assert.Contains(t, got, useNodeKey("lane.src", "lane.dst"),
		"a foreign edge bleeding through the lone inline label must flag its edge for node mode")
}

func inlineLabelSVGBox(x, y float64, label string, ruler *textmeasure.Ruler) labelBox {
	boxes := inlineLabelBoxes([]byte(syntheticBleedSVG(x, y, label, x-1000)), ruler)
	if len(boxes) != 1 {
		panic(fmt.Sprintf("synthetic SVG must yield exactly one inline label box, got %d", len(boxes)))
	}

	return boxes[0]
}

func syntheticBleedSVG(x, y float64, label string, foreignX float64) string {
	return fmt.Sprintf(
		`<svg>`+
			`<g transform="translate(%.6f %.6f)" class="light-code"><text x="0" y="12">%s</text></g>`+
			`<path d="M%.6f %.6f L%.6f %.6f" class="connection" />`+
			`</svg>`,
		x, y, label,
		foreignX, y-20, foreignX, y+40,
	)
}
