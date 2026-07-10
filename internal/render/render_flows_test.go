package render

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func TestRenderFlowDeterministic(t *testing.T) {
	t.Parallel()

	f := &Flow{ID: "x", Nodes: []*Node{
		{Flow: "x", Qual: "worker.A", Lane: "worker", Pos: "f.go:1"},
		{Flow: "x", Qual: "api.B", Lane: "api", Pos: "f.go:2"},
	}, Edges: []Edge{
		{From: "worker.A", To: "api.B", Note: "n;with:specials"},
	}}

	first := renderFlow(f, nil, testShapes)
	assert.Equal(t, first, renderFlow(f, nil, testShapes), "renderFlow must be deterministic")

	assert.Contains(t, first, "lane_worker.A -> lane_api.B: |txt n;with:specials |", "edge note is an inline txt label")
	assert.NotContains(t, first, "lbl_A_B", "default render adds no label node")
	assert.NotContains(t, first, "class: elabel", "default render uses no elabel node")
	assert.Contains(t, first, "style.text-transform: none", "inline label opts out of CapsLock")

	assert.Contains(t, first, "lane_worker: worker {", "worker lane container")
	assert.Contains(t, first, "lane_api: api {", "api lane container")

	_, err := compileSVG(first)
	require.NoError(t, err, "rendered flow must compile")

	node := renderFlow(f, map[string]bool{useNodeKey("worker.A", "api.B"): true}, testShapes)
	assert.Contains(t, node, "lane_worker.lbl_A_B: |txt n;with:specials | {", "node mode: edge note is an elabel txt node")
	assert.Contains(t, node, "class: elabel", "node mode: label node carries the elabel class")
	assert.Contains(t, node, "lane_worker.A -> lane_worker.lbl_A_B: {", "node mode: src -> labelNode segment")
	assert.Contains(t, node, "target-arrowhead.shape: none", "node mode: src segment drops its arrowhead")
	assert.Contains(t, node, "lane_worker.lbl_A_B -> lane_api.B", "node mode: labelNode -> dst carries the arrow")
}

func TestRenderFlowEdgeCondDistinctFromNote(t *testing.T) {
	t.Parallel()

	f := &Flow{ID: "x", Nodes: []*Node{
		{Flow: "x", Qual: "api.Lock", Kind: KindDecision, Lane: "api", Pos: "f.go:1"},
		{Flow: "x", Qual: "api.Go", Lane: "api", Pos: "f.go:2"},
	}, Edges: []Edge{
		{From: "api.Lock", To: "api.Go", Cond: "won lock", Note: "first caller wins"},
	}}

	out := renderFlow(f, nil, testShapes)

	wantInline := "lane_api.Lock -> lane_api.Go: |txt\n  won lock\n  first caller wins\n|"
	assert.Contains(t, out, wantInline, "cond over note in one inline txt label")
	assert.NotContains(t, out, "lbl_Lock_Go", "default render adds no label node")
	assert.NotContains(t, out, "tooltip:", "no tooltips — they don't render as readable text")
	assert.Contains(t, out, "style.bold: true", "a cond-bearing label is bold")

	node := renderFlow(f, map[string]bool{useNodeKey("api.Lock", "api.Go"): true}, testShapes)
	wantLabelNode := "lane_api.lbl_Lock_Go: |txt\n  won lock\n  first caller wins\n| {"
	assert.Contains(t, node, wantLabelNode, "node mode: cond over note in one txt label node")
	assert.Contains(t, node, "lane_api.Lock -> lane_api.lbl_Lock_Go: {", "node mode: src -> labelNode segment")
	assert.Contains(t, node, "lane_api.lbl_Lock_Go -> lane_api.Go", "node mode: labelNode -> dst carries the arrow")
	assert.Contains(t, node, "style.bold: true", "node mode: a cond-bearing label node is bold")
}

func TestRenderGoroutineEdgeIsDashed(t *testing.T) {
	t.Parallel()

	async := dumpEdge("lane_feed.Connect", "lane_feed.sessionLoop",
		Edge{From: "feed.Connect", To: "feed.sessionLoop", Goroutine: true}, false)
	assert.Contains(t, async, "style.stroke-dash: 3", "a label-less goroutine edge is dashed")

	seq := dumpEdge("lane_feed.sessionLoop", "lane_feed.runOneSession",
		Edge{From: "feed.sessionLoop", To: "feed.runOneSession"}, false)
	assert.NotContains(t, seq, "stroke-dash", "a sequential edge is not dashed")
	assert.NotContains(t, seq, ": {", "a sequential label-less edge gets no style block (byte-neutral)")

	e := Edge{From: "feed.Connect", To: "feed.sessionLoop", Note: "go sessionLoop", Goroutine: true}

	inline := dumpEdge("lane_feed.Connect", "lane_feed.sessionLoop", e, false)
	assert.Contains(t, inline, "style.stroke-dash: 3", "an inline-labeled goroutine edge is dashed")

	node := dumpEdge("lane_feed.Connect", "lane_feed.sessionLoop", e, true)
	assert.Contains(t, node, "lane_feed.Connect -> lane_feed.lbl_Connect_sessionLoop",
		"node mode: src->label segment present")
	assert.Contains(t, node, "lane_feed.lbl_Connect_sessionLoop -> lane_feed.sessionLoop",
		"node mode: label->dst segment present")
	assert.Equal(t, 2, strings.Count(node, "stroke-dash"),
		"node mode: both split segments are dashed (one continuous dashed arrow)")
}

func TestWrapLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		wantLines int
	}{
		{
			name:      "short label is untouched",
			in:        "first caller wins",
			wantLines: 1,
		},
		{
			name:      "a long label wraps onto multiple lines",
			in:        "writes record FIRST, then inserts into the shared map next",
			wantLines: 2,
		},
		{
			name:      "a single word longer than the width is returned whole, never broken",
			in:        "api.ItemController.ReplaceItemsWithAVeryLongSuffixHere",
			wantLines: 1,
		},
		{
			name:      "an over-width first word keeps following words on later lines",
			in:        "supercalifragilisticexpialidociousveryverylongword then short",
			wantLines: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := wrapLabel(tc.in)
			lines := strings.Split(got, "\n")

			assert.Len(t, lines, tc.wantLines, "line count for %q", tc.in)

			assert.Equal(t, strings.Fields(tc.in), strings.Fields(got),
				"wrapLabel must only insert newlines at word boundaries")

			words := strings.Fields(tc.in)
			wi := 0

			for li, line := range lines {
				lineWords := strings.Fields(line)

				overWidthLoneWord := len(lineWords) == 1 && len(lineWords[0]) > wrapWidth
				if !overWidthLoneWord {
					assert.LessOrEqual(t, len(line), wrapWidth, "line %d of %q over width", li, tc.in)
				}

				if li > 0 && wi < len(words) {
					prev := lines[li-1]
					assert.Greater(t, len(prev)+1+len(words[wi]), wrapWidth,
						"line %d's first word should have packed onto the previous line", li)
				}

				wi += len(lineWords)
			}
		})
	}

	out := dumpEdge("lane_a.x", "lane_b.y", Edge{
		Cond: "won the write race",
		Note: "writes record FIRST, then inserts into the shared map next",
	}, false)

	assert.Contains(t, out, "|txt\n", "long edge label is a txt block (carries newlines)")
	assert.Regexp(t, `\n\s*won the write race\n`, out, "cond is the first label line")
	assert.Contains(t, out, "\n  writes record FIRST, then inserts into the\n",
		"the long note wraps at the width boundary inside the txt block")
	assert.Contains(t, out, "\n  shared map next\n", "the wrapped remainder is its own txt-block line")
}

func TestHaloLabels(t *testing.T) {
	t.Parallel()

	inline := `<g transform="translate(12.000000 251.000000)" class="light-code">` +
		`<text class="text-mono" x="0" y="1.000000em">inline label</text></g>`
	node := `<g transform="translate(62.000000 208.000000)" class="light-code">` +
		`<rect width="176" height="36" stroke="#000410" class="shape stroke-N1" style="fill:#FFFFFF;stroke:#000410;stroke-width:2;" />` +
		`<g transform="translate(8 8)"><text class="text-mono" x="0" y="1.000000em">node label</text></g></g>`
	title := `<text x="430" fill="#000410" class="text-mono fill-N1" style="font-size:28px">lane title</text>`

	out := string(haloLabels([]byte(inline + node + title)))

	assert.Equal(t, 1, strings.Count(out, `paint-order="stroke"`),
		"only the lane title is haloed (inline and node labels get opaque chips instead)")
	assert.Contains(t, out, `stroke="`+laneFill+`"`, "halo is the lane-background color")
	assert.Contains(t, out, `stroke-width="6px"`, "lane title gets the title halo width")

	assert.Contains(t, out, `<g transform="translate(8 8)"><text class="text-mono" x="0" y="1.000000em">node label</text></g>`,
		"node-mode edge labels are left un-haloed (opaque chip masks the line instead)")

	assert.Equal(t, out, string(haloLabels([]byte(out))), "haloLabels must be idempotent")
}

func TestClearTitleX(t *testing.T) {
	t.Parallel()

	const halfW = 74.5

	unbounded := func(crossing []float64) (float64, bool) {
		return clearTitleX(543.5, halfW, crossing, math.Inf(-1), math.Inf(1))
	}

	x, moved := unbounded([]float64{900})
	assert.False(t, moved, "an edge outside the span needs no shift")
	assert.InDelta(t, 543.5, x, 0.01)

	x, moved = unbounded([]float64{476.5})
	assert.True(t, moved, "an edge inside the span forces a shift")
	assert.GreaterOrEqual(t, x-halfW, 476.5+titleClear-0.01,
		"title left edge clears the crossing edge")

	x, moved = unbounded([]float64{600})
	assert.True(t, moved)
	assert.LessOrEqual(t, x+halfW, 600-titleClear+0.01,
		"title right edge clears the crossing edge")

	crossing := []float64{440, 500, 560}
	x, moved = clearTitleX(500, halfW, crossing, math.Inf(-1), math.Inf(1))
	assert.True(t, moved, "an edge cluster spanning the center forces a shift")

	for _, e := range crossing {
		isClear := e <= x-halfW-titleClear+0.01 || e >= x+halfW+titleClear-0.01
		assert.True(t, isClear, "every crossing edge (incl. middle) is clear of the moved title; edge=%v span=[%v,%v]",
			e, x-halfW, x+halfW)
	}

	x, moved = clearTitleX(543.5, halfW, []float64{540}, math.Inf(-1), 600)
	assert.True(t, moved, "when the nearer side is out of bounds the other side is used")
	assert.LessOrEqual(t, x+halfW, 540-titleClear+0.01, "title shifted left instead")

	x, moved = clearTitleX(543.5, halfW, []float64{543}, 500, 600)
	assert.False(t, moved, "when neither side fits in the lane the title stays put")
	assert.InDelta(t, 543.5, x, 0.01)
}

func TestVerticalSegmentsToleratesELKJitter(t *testing.T) {
	t.Parallel()

	svg := []byte(`<path d="M 117.002257 150.999999 L 117.495485 588.000003" class="connection"` +
		`<path d="M 200.0 100.0 L 240.0 500.0" class="connection"`)

	segs := verticalSegments(svg)

	assert.Len(t, segs, 1, "the near-vertical segment is kept, the diagonal one is not")
	assert.InDelta(t, 117.25, segs[0].x, 0.01, "x is the segment midpoint")
	assert.InDelta(t, 151, segs[0].yTop, 0.01)
	assert.InDelta(t, 588, segs[0].yBot, 0.01)
}

func TestMinLaneWidthFitsTitleBesideCenterEdge(t *testing.T) {
	t.Parallel()

	w := float64(minLaneWidth("worker"))
	titleW := 6 * titleRunePx

	assert.GreaterOrEqual(t, w/2, titleW+titleClear+laneTitleInset,
		"half the lane fits the shifted title plus clearance when an edge crosses dead-center")
}

func TestPlaceLaneTitlesPadsBorderHuggingTitle(t *testing.T) {
	t.Parallel()

	ruler, err := textmeasure.NewRuler()
	require.NoError(t, err)

	lane := `<rect x="12.000000" y="542.000000" width="310.000000" height="354.000000" stroke="#0000E4" fill="` +
		laneFill + `" />`
	title := `<text x="67.000000" y="575.000000" fill="#000410" class="text-mono fill-N1" ` +
		`style="text-anchor:middle;font-size:28px">worker</text>`

	out := string(placeLaneTitles([]byte(lane+title), ruler))

	m := regexp.MustCompile(`<text x="([\d.]+)"[^>]*>worker</text>`).FindStringSubmatch(out)
	require.NotNil(t, m)

	font := d2fonts.SourceCodePro.Font(titleFontPx, d2fonts.FONT_STYLE_REGULAR)
	w, _ := ruler.Measure(font, "worker")

	x, _ := strconv.ParseFloat(m[1], 64)
	assert.InDelta(t, 12+laneTitleInset+float64(w)/2, x, 0.01,
		"a border-hugging title is nudged in to the lane inset")
}

func TestSizeRoot(t *testing.T) {
	t.Parallel()

	in := []byte(`<svg xmlns="..." data-d2-version="0.7.1" preserveAspectRatio="xMinYMin meet" viewBox="0 0 607 900">` +
		`<svg class="d2-svg" width="607" height="900" viewBox="-9 -9 607 900"></svg></svg>`)

	out := string(sizeRoot(in))

	assert.Contains(t, out, `viewBox="0 0 607 900" width="607" height="900">`,
		"the outer wrapper gets width/height from its viewBox")

	assert.Equal(t, 1, strings.Count(out, `width="607" height="900">`),
		"only the outer wrapper is sized, not the inner d2-svg")

	assert.Equal(t, out, string(sizeRoot([]byte(out))), "sizeRoot must be idempotent")
}

func TestRenderNodeKindShape(t *testing.T) {
	t.Parallel()

	store := dumpNode(&Node{Qual: "worker.Fold", Kind: KindStore, Lane: "worker", Pos: "f.go:9"})
	assert.Contains(t, store, "class: store")
	assert.Contains(t, store, "Fold: Fold {", "shaped node label is the bare func name")
	assert.NotContains(t, store, "tooltip:", "shaped node must not emit a tooltip")
	assert.NotContains(t, store, "worker.Fold", "the full qual lives in the .md step list, not the shaped node")
	assert.NotContains(t, store, "f.go:9", "node must not embed the line number")

	plain := dumpNode(&Node{Qual: "worker.Plain", Lane: "worker", Pos: "f.go:3"})
	assert.NotContains(t, plain, "|md", "default node must not use |md (renders as an invisible foreignObject)")
	assert.Contains(t, plain, "Plain: Plain {", "default node label is plain text, not markdown")
	assert.Contains(t, plain, "shape: rectangle", "default node gets a visible box border")
	assert.NotContains(t, plain, "class:")
	assert.NotContains(t, plain, "f.go:3", "node must not embed the line number")
}

func TestRenderMarkdownReferencesSVGInSubdir(t *testing.T) {
	t.Parallel()

	f := &Flow{ID: "demo", Nodes: []*Node{
		{Flow: "demo", Qual: "worker.A", Lane: "worker", Pos: "pkg/x.go:1"},
	}}

	md := renderMarkdown(f, "svg/demo.svg", 420, "x -> y", "docs/flows")

	assert.Contains(t, md, `<img src="svg/demo.svg"`)
	assert.Contains(t, md, `width="420"`, "img must be width-capped so it doesn't balloon")
	assert.NotContains(t, md, "<svg", "SVG must be a separate file, not inlined")
	assert.Contains(t, md, "../../pkg/x.go#L1", "source links climb from the output dir back to the module root")
	assert.Contains(t, md, "go generate ./docs/flows", "the regen hint names the output dir")
}

func TestRenderMarkdownLinkDepthFollowsOutDir(t *testing.T) {
	t.Parallel()

	f := &Flow{ID: "demo", Nodes: []*Node{{Flow: "demo", Qual: "worker.A", Lane: "worker", Pos: "pkg/x.go:1"}}}

	md := renderMarkdown(f, "svg/demo.svg", 420, "x -> y", "docs")

	assert.Contains(t, md, "../pkg/x.go#L1", "a shallower output dir uses a shallower link prefix")
	assert.Contains(t, md, "go generate ./docs", "the regen hint tracks the output dir")
}

func TestSrcPrefix(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "../", srcPrefix("docs"))
	assert.Equal(t, "../../", srcPrefix("docs/flows"))
	assert.Equal(t, "../../../", srcPrefix("a/b/c"))
	assert.Equal(t, "../", srcPrefix("./docs"), "a leading ./ is cleaned away")
}
