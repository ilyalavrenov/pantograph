package render

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"unicode/utf8"

	"github.com/ilyalavrenov/pantograph/internal/collect"
	"oss.terrastruct.com/d2/d2graph"
	"oss.terrastruct.com/d2/d2layouts/d2elklayout"
	"oss.terrastruct.com/d2/d2lib"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/d2renderers/d2svg"
	"oss.terrastruct.com/d2/d2themes/d2themescatalog"
	d2log "oss.terrastruct.com/d2/lib/log"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

func Render(flows map[string]*collect.Flow, shapes map[string]string, outDir string) (map[string]string, error) {
	ids := make([]string, 0, len(flows))
	for id := range flows {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	defer debug.SetGCPercent(debug.SetGCPercent(batchGCPercent))

	results := make([]flowResult, len(ids))

	cache := newCompileCache()

	var wg sync.WaitGroup

	for i := range ids {
		wg.Go(func() {
			results[i] = renderOne(flows[ids[i]], ids[i], cache, shapes, outDir)
		})
	}

	wg.Wait()

	files := make(map[string]string, len(ids)+1)

	for i, id := range ids {
		if results[i].err != nil {
			return nil, results[i].err
		}

		files[svgRef(id)] = results[i].svg
		files[id+".md"] = results[i].md
	}

	files["index.md"] = renderIndex(ids, flows, outDir)

	return files, nil
}

type flowResult struct {
	md, svg string
	err     error
}

const maxImgWidth = 880

const batchGCPercent = 400

func svgWidth(svg []byte) int {
	m := svgWidthRe.FindSubmatch(svg)
	if m == nil {
		return maxImgWidth
	}

	w, err := strconv.Atoi(string(m[1]))
	if err != nil || w <= 0 {
		return maxImgWidth
	}

	return w
}

var svgWidthRe = regexp.MustCompile(`width="(\d+)"`)

func svgHeight(svg []byte) int {
	m := svgHeightRe.FindSubmatch(svg)
	if m == nil {
		return 0
	}

	h, err := strconv.Atoi(string(m[1]))
	if err != nil || h <= 0 {
		return 0
	}

	return h
}

var svgHeightRe = regexp.MustCompile(`height="(\d+)"`)

const targetAspectRatio = 16.0 / 9.0

const githubColumnWidth = maxImgWidth

func shapeScore(w, h int) float64 {
	if h <= 0 || w <= 0 {
		return math.Inf(1)
	}

	aspect := math.Abs(math.Log(float64(w)/float64(h)) - math.Log(targetAspectRatio))

	overflow := 0.0
	if w > githubColumnWidth {
		overflow = math.Log(float64(w) / float64(githubColumnWidth))
	}

	return aspect + overflow
}

func aspectScore(svg []byte) float64 {
	return shapeScore(svgWidth(svg), svgHeight(svg))
}

const maxCollisionPasses = 5

func renderOne(f *collect.Flow, id string, cache *compileCache, shapes map[string]string, outDir string) flowResult {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return flowResult{err: fmt.Errorf("new ruler for flow %q: %w", id, err)}
	}

	orders := append([][]*collect.Node{f.Nodes}, reorderCandidates(f.Nodes, f.Edges)...)

	results := make([]candResult, len(orders))

	var wg sync.WaitGroup

	for i, order := range orders {
		wg.Go(func() {
			cand := &collect.Flow{ID: f.ID, Nodes: order, Edges: f.Edges, Members: f.Members, Note: f.Note}

			candRuler, rErr := textmeasure.NewRuler()
			if rErr != nil {
				results[i] = candResult{err: fmt.Errorf("new ruler for flow %q: %w", id, rErr)}

				return
			}

			var (
				r   candidateRender
				err error
			)

			withCompileSlot(func() {
				r, err = resolveCollisions(cand, id, candRuler, cache, shapes)
			})
			if err != nil {
				results[i] = candResult{err: err}

				return
			}

			r.order = i
			results[i] = candResult{r: r}
		})
	}

	wg.Wait()

	best, err := selectBestCandidate(results)
	if err != nil {
		return flowResult{err: err}
	}

	if best == nil {
		return flowResult{err: fmt.Errorf("flow %q: no candidate rendered", id)}
	}

	finalSVG := chipInlineLabels([]byte(best.svg), ruler)

	width := svgWidth(finalSVG)

	return flowResult{md: renderMarkdown(f, svgRef(id), width, best.src, outDir), svg: string(finalSVG)}
}

func selectBestCandidate(results []candResult) (*candidateRender, error) {
	var best *candidateRender

	for i, res := range results {
		if res.err != nil {
			if i == 0 {
				return nil, res.err
			}

			continue
		}

		r := res.r
		if best == nil || r.better(best) {
			best = &r
		}
	}

	return best, nil
}

type candResult struct {
	r   candidateRender
	err error
}

type candidateRender struct {
	src, svg                    string
	aspectScore                 float64
	crossings, nodeCount, order int
}

func (r candidateRender) better(best *candidateRender) bool {
	if r.crossings != best.crossings {
		return r.crossings < best.crossings
	}

	if r.aspectScore != best.aspectScore {
		return r.aspectScore < best.aspectScore
	}

	if r.nodeCount != best.nodeCount {
		return r.nodeCount < best.nodeCount
	}

	return r.order < best.order
}

func resolveCollisions(
	cand *collect.Flow, id string, ruler *textmeasure.Ruler, cache *compileCache,
	shapes map[string]string,
) (candidateRender, error) {
	useNode := map[string]bool{}

	for pass := 0; ; pass++ {
		src := renderFlow(cand, useNode, shapes)

		svg, err := cache.compile(src)
		if err != nil {
			return candidateRender{}, fmt.Errorf("compile flow %q: %w", id, err)
		}

		added := false

		if pass < maxCollisionPasses {
			for k := range collidingEdgeKeys(svg, cand, ruler) {
				if !useNode[k] {
					useNode[k] = true
					added = true
				}
			}
		}

		if !added {
			return candidateRender{
				src:         src,
				svg:         string(svg),
				crossings:   countEdgeCrossings(parseConnectionPaths(svg)),
				nodeCount:   len(useNode),
				aspectScore: aspectScore(svg),
			}, nil
		}

		if pass+1 == maxCollisionPasses {
			fmt.Fprintf(os.Stderr, "pantograph: warning: inline-label collisions unresolved after %d passes for flow %q (%d label-nodes)\n",
				pass+1, id, len(useNode))
		}
	}
}

const svgSubdir = "svg"

func svgRef(id string) string {
	return svgSubdir + "/" + id + ".svg"
}

func WriteFiles(log *slog.Logger, files map[string]string, outDir string) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(outDir, name)

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:mnd // standard directory permission
			return fmt.Errorf("mkdir for %s: %w", path, err)
		}

		if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil { //nolint:gosec,mnd // world-readable generated docs
			return fmt.Errorf("write %s: %w", path, err)
		}

		log.Info("wrote " + path)
	}

	return nil
}

func firstDiff(got, want string) string {
	split := func(s string) []string {
		if strings.Contains(s, "</svg>") {
			return strings.SplitAfter(s, ">")
		}

		return strings.Split(s, "\n")
	}

	g, w := split(got), split(want)

	for i := 0; i < len(g) || i < len(w); i++ {
		var gi, wi string
		if i < len(g) {
			gi = g[i]
		}

		if i < len(w) {
			wi = w[i]
		}

		if gi != wi {
			const ctx = 200

			return fmt.Sprintf("    first diff at unit %d:\n      on disk:  %s\n      rendered: %s",
				i, trunc(gi, ctx), trunc(wi, ctx))
		}
	}

	return "    (content length differs but no unit diff found)"
}

func trunc(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}

	return s
}

func CheckUpToDate(files map[string]string, outDir string) error {
	var stale []string

	for name, want := range files {
		path := filepath.Join(outDir, name)

		got, err := os.ReadFile(path)
		if err != nil {
			stale = append(stale, fmt.Sprintf("  %s: missing or unreadable (%v)", path, err))

			continue
		}

		if string(got) != want {
			stale = append(stale, fmt.Sprintf("  %s: out of date\n%s", path, firstDiff(string(got), want)))
		}
	}

	stale = append(stale, orphans(outDir, "", files)...)
	stale = append(stale, orphans(filepath.Join(outDir, svgSubdir), svgSubdir+"/", files)...)

	if len(stale) > 0 {
		sort.Strings(stale)

		return fmt.Errorf("flow diagrams are out of date — run `go generate ./%s`:\n%s",
			outDir, strings.Join(stale, "\n"))
	}

	return nil
}

func orphans(dir, keyPrefix string, files map[string]string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var out []string

	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || (!strings.HasSuffix(n, ".md") && !strings.HasSuffix(n, ".svg")) {
			continue
		}

		if _, ok := files[keyPrefix+n]; !ok {
			out = append(out, fmt.Sprintf("  %s: orphaned (no matching flow)", filepath.Join(dir, n)))
		}
	}

	return out
}

func compileSVG(d2src string) ([]byte, error) {
	ruler, err := textmeasure.NewRuler()
	if err != nil {
		return nil, fmt.Errorf("new ruler: %w", err)
	}

	layoutResolver := func(string) (d2graph.LayoutGraph, error) {
		return d2elklayout.DefaultLayout, nil
	}

	svgPad := int64(20) //nolint:mnd // layout constant, not a magic number
	omitVersion := true
	renderOpts := &d2svg.RenderOpts{
		Pad:         &svgPad,
		ThemeID:     &d2themescatalog.Terminal.ID,
		OmitVersion: &omitVersion,
	}
	compileOpts := &d2lib.CompileOptions{
		LayoutResolver: layoutResolver,
		Ruler:          ruler,
	}

	ctx := d2log.With(context.Background(), slog.New(slog.DiscardHandler))

	diagram, _, err := d2lib.Compile(ctx, d2src, compileOpts, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("d2 compile: %w", err)
	}

	svg, err := d2svg.Render(diagram, renderOpts)
	if err != nil {
		return nil, fmt.Errorf("d2 render: %w", err)
	}

	return sizeRoot(haloLabels(placeLaneTitles(styleLabelNodeBackings(svg), ruler))), nil
}

var labelNodeBacking = regexp.MustCompile(
	`(<g[^>]*class="light-code"><rect\b[^>]*?style=")fill:#ffffff;stroke-width:0;"`)

const (
	labelChipFill     = "#FFFFFF"
	labelChipStroke   = "#000410"
	labelChipStrokePx = 2
)

func styleLabelNodeBackings(svg []byte) []byte {
	repl := fmt.Sprintf(`${1}fill:%s;stroke:%s;stroke-width:%d;"`, labelChipFill, labelChipStroke, labelChipStrokePx)

	return labelNodeBacking.ReplaceAll(svg, []byte(repl))
}

const chipTextInset = 8

func chipInlineLabels(svg []byte, ruler *textmeasure.Ruler) []byte {
	font := d2fonts.SourceCodePro.Font(edgeLabelFontPx, d2fonts.FONT_STYLE_REGULAR)

	return inlineLabelGroup.ReplaceAllFunc(svg, func(group []byte) []byte {
		m := inlineLabelGroup.FindSubmatch(group)
		body := m[3]

		var (
			maxW   float64
			nLines int
		)

		for _, l := range inlineLineText.FindAllSubmatch(body, -1) {
			w, _ := ruler.Measure(font, decodeSVGText(string(l[1])))
			maxW = max(maxW, float64(w))
			nLines++
		}

		if nLines == 0 {
			return group
		}

		chipX := -float64(chipTextInset)
		chipY := edgeLabelFontPx - labelAscentPx - float64(chipTextInset)
		chipW := maxW + 2*chipTextInset
		chipH := labelAscentPx + float64(nLines-1)*labelLineHeight + labelDescentPx + 2*chipTextInset

		rect := fmt.Appendf(nil,
			`<rect x="%.6f" y="%.6f" width="%.6f" height="%.6f" fill="%s" stroke="%s" stroke-width="%d" />`,
			chipX, chipY, chipW, chipH, labelChipFill, labelChipStroke, labelChipStrokePx)

		open := group[:len(group)-len(body)-len("</g>")]

		return append(append(append(append([]byte(nil), open...), rect...), body...), "</g>"...)
	})
}

const (
	labelAscentPx  = 12
	labelDescentPx = 4
)

const (
	edgeLabelFontPx = 16
	labelLineHeight = edgeLabelFontPx * 1.3
)

var rootSVGOpen = regexp.MustCompile(`(<svg [^>]*preserveAspectRatio="[^"]*" viewBox="0 0 (\d+) (\d+)")>`)

func sizeRoot(svg []byte) []byte {
	return rootSVGOpen.ReplaceAll(svg, []byte(`$1 width="$2" height="$3">`))
}

const laneFill = "#E7E9EE"

func haloHTML(strokeWidth int) string {
	return fmt.Sprintf(` paint-order="stroke" stroke="%s" stroke-width="%dpx" stroke-linejoin="round">`,
		laneFill, strokeWidth)
}

const titleHaloPx = 6

var titleTextOpen = regexp.MustCompile(`<text [^>]*class="text-mono[ "][^>]*font-size:28px[^>]*>`)

func haloLabels(svg []byte) []byte {
	return titleTextOpen.ReplaceAllFunc(svg, func(tag []byte) []byte {
		return spliceHalo(tag, titleHaloPx)
	})
}

func spliceHalo(tag []byte, width int) []byte {
	if bytes.Contains(tag, []byte("paint-order")) {
		return tag
	}

	return append(tag[:len(tag)-1:len(tag)-1], haloHTML(width)...)
}

var laneTitleText = regexp.MustCompile(
	`<text x="([\d.]+)" y="([\d.]+)"[^>]*font-size:28px[^>]*>([^<]+)</text>`)

var connectionPath = regexp.MustCompile(`<path d="(M[^"]+)"[^>]*class="connection`)

const (
	titleFontPx = 28
	titleClear  = 30.0
	// Buffer between a title's edge and the lane rect edge: the Terminal
	// theme's double dash renders ~8px inside the rect, so anything under
	// ~12px reads as touching the border.
	laneTitleInset = 20.0
)

func placeLaneTitles(svg []byte, ruler *textmeasure.Ruler) []byte {
	font := d2fonts.SourceCodePro.Font(titleFontPx, d2fonts.FONT_STYLE_REGULAR)
	vsegs := verticalSegments(svg)
	lanes := laneBorderRects(svg)

	return laneTitleText.ReplaceAllFunc(svg, func(tag []byte) []byte {
		m := laneTitleText.FindSubmatch(tag)
		if m == nil {
			return tag
		}

		cx := parseF(m[1])
		cy := parseF(m[2])
		w, h := ruler.Measure(font, decodeSVGText(string(m[3])))
		halfW := float64(w) / 2

		bandTop := cy - float64(h)
		crossing := edgesInBand(vsegs, bandTop, cy)

		loX, hiX := laneSpan(lanes, cx, cy)
		loCX, hiCX := loX+laneTitleInset+halfW, hiX-laneTitleInset-halfW

		// d2 places top-left container labels nearly on the border; pad them in.
		newCX := max(cx, loCX)
		if shifted, moved := clearTitleX(newCX, halfW, crossing, loCX, hiCX); moved {
			newCX = shifted
		}

		if newCX == cx {
			return tag
		}

		oldX := fmt.Sprintf(`x="%s"`, m[1])
		newX := fmt.Sprintf(`x="%.6f"`, newCX)

		return bytes.Replace(tag, []byte(oldX), []byte(newX), 1)
	})
}

func parseF(b []byte) float64 {
	f, _ := strconv.ParseFloat(string(b), 64) //nolint:errcheck // d2-generated coord, always a valid float

	return f
}

type segment struct{ x, yTop, yBot float64 }

// ELK emits "vertical" segments with sub-pixel x drift (~0.5px over long runs);
// exact equality made them invisible to every collision check.
const vertTolPx = 2.0

func verticalSegments(svg []byte) []segment {
	var out []segment

	for _, m := range connectionPath.FindAllSubmatch(svg, -1) {
		pts := pathPoint.FindAllSubmatch(m[1], -1)
		for i := 1; i < len(pts); i++ {
			x0, y0 := parseF(pts[i-1][1]), parseF(pts[i-1][2])
			x1, y1 := parseF(pts[i][1]), parseF(pts[i][2])

			if math.Abs(x0-x1) <= vertTolPx && y0 != y1 {
				out = append(out, segment{x: (x0 + x1) / 2, yTop: min(y0, y1), yBot: max(y0, y1)})
			}
		}
	}

	return out
}

var pathPoint = regexp.MustCompile(`([\d.]+) ([\d.]+)`)

func edgesInBand(segs []segment, top, bot float64) []float64 {
	seen := map[float64]bool{}

	var xs []float64

	for _, s := range segs {
		if s.yTop <= bot && s.yBot >= top && !seen[s.x] {
			seen[s.x] = true

			xs = append(xs, s.x)
		}
	}

	sort.Float64s(xs)

	return xs
}

func laneSpan(lanes []laneRect, x, y float64) (float64, float64) {
	for _, l := range lanes {
		if x >= l.x && x <= l.x+l.w && y >= l.y && y <= l.y+l.h {
			return l.x, l.x + l.w
		}
	}

	return math.Inf(-1), math.Inf(1)
}

// loCX/hiCX bound the title's center; a shift that would poke outside its lane
// is rejected and the title stays put (z-order + halo keep it legible).
func clearTitleX(cx, halfW float64, crossing []float64, loCX, hiCX float64) (float64, bool) {
	inSpan := false

	for _, x := range crossing {
		if x > cx-halfW && x < cx+halfW {
			inSpan = true

			break
		}
	}

	if !inSpan {
		return cx, false
	}

	leftEdge, rightEdge := crossing[0], crossing[len(crossing)-1]

	right := rightEdge + titleClear + halfW
	left := leftEdge - titleClear - halfW

	rightOK := right <= hiCX
	leftOK := left >= loCX

	switch {
	case rightOK && (!leftOK || right-cx <= cx-left):
		return right, true
	case leftOK:
		return left, true
	default:
		return cx, false
	}
}

func decodeSVGText(s string) string {
	r := strings.NewReplacer(
		"&#160;", " ",
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	)

	return strings.TrimSpace(r.Replace(s))
}

func nodeID(label string) string {
	var b strings.Builder

	for _, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '/':
			b.WriteByte('_')
		}
	}

	return b.String()
}

func posLink(pos string) string {
	if i := strings.LastIndex(pos, ":"); i >= 0 {
		return pos[:i] + "#L" + pos[i+1:]
	}

	return pos
}

func useNodeKey(from, to string) string { return from + "->" + to }

const (
	// Source Code Pro advance at the 28px title size (0.6 em).
	titleRunePx    = 16.8
	laneWidthSlack = 8.0
)

// Min-width floor (ELK NodeSizeMinimum) so the lane title can clear a crossing
// edge on at least one side even when the edge runs dead-center; without it,
// narrow lanes leave placeLaneTitles nowhere to move the title.
func minLaneWidth(lane string) int {
	titleW := float64(utf8.RuneCountInString(lane)) * titleRunePx

	return int(math.Ceil(2*(titleW+titleClear+laneTitleInset) + laneWidthSlack))
}

func renderFlow(f *collect.Flow, useNode map[string]bool, shapes map[string]string) string {
	root := newD2Map()
	root.comment("Code-generated by pantograph. DO NOT EDIT.")
	root.comment("Domain: " + f.ID + " (fuses " + strings.Join(f.Members, ", ") +
		")\nSource of truth: //pantograph: annotations in the Go source.")
	root.set("direction", uq("down"))
	addClasses(root, shapes)

	lanes := sortedLanes(f.Nodes)

	qid := make(map[string]string, len(f.Nodes))
	for _, n := range f.Nodes {
		qid[n.Qual] = laneNodeID(n)
	}

	for _, lane := range lanes {
		container := newD2Map()
		container.set("class", uq("lane"))
		container.set("width", uq(strconv.Itoa(minLaneWidth(lane))))
		container.setPath([]string{"label", "near"}, uq("top-left"))

		for _, n := range f.Nodes {
			if n.Lane != lane {
				continue
			}

			addNode(container, n, shapes)
		}

		root.labeledChild(laneID(lane), strVal(lane), container)
	}

	for _, e := range orderedEdges(f) {
		addEdge(root, qid[e.From], qid[e.To], e, useNode[useNodeKey(e.From, e.To)])
	}

	return root.format()
}

func orderedEdges(f *collect.Flow) []collect.Edge {
	rank := columnRanks(f)

	edges := make([]collect.Edge, len(f.Edges))
	copy(edges, f.Edges)

	sort.SliceStable(edges, func(i, j int) bool {
		if r := rank[edges[i].From] - rank[edges[j].From]; r != 0 {
			return r < 0
		}

		return rank[edges[i].To] < rank[edges[j].To]
	})

	return edges
}

func columnRanks(f *collect.Flow) map[string]int {
	laneIdx := make(map[string]int)
	for i, lane := range sortedLanes(f.Nodes) {
		laneIdx[lane] = i
	}

	stride := len(f.Nodes) + 1

	rank := make(map[string]int, len(f.Nodes))
	for i, n := range f.Nodes {
		rank[n.Qual] = laneIdx[n.Lane]*stride + i
	}

	return rank
}

func edgeLabel(e collect.Edge) string {
	switch {
	case e.Cond != "" && e.Note != "":
		return wrapLabel(e.Cond) + "\n" + wrapLabel(e.Note)
	case e.Cond != "":
		return wrapLabel(e.Cond)
	case e.Note != "":
		return wrapLabel(e.Note)
	default:
		return ""
	}
}

func addEdge(root *d2Map, from, to string, e collect.Edge, useNode bool) {
	label := edgeLabel(e)
	if label == "" {
		root.edge(splitID(from), splitID(to), nil, markGoroutine(newD2Map(), e))

		return
	}

	if !useNode {
		addEdgeInline(root, from, to, e, label)

		return
	}

	labelID := edgeLabelNodeID(from, to)

	labelNode := newD2Map()
	labelNode.set("class", uq(edgeLabelClass))

	if e.Cond != "" {
		labelNode.setPath([]string{d2Style, "bold"}, uq("true"))
	}

	labelNode.set("width", uq(strconv.Itoa(labelNodeWidth(label))))
	labelNode.set("height", uq(strconv.Itoa(labelNodeHeight(label))))

	root.labeledChildPath(splitID(labelID), blockStr("txt", label), labelNode)

	noArrow := newD2Map()
	noArrow.setPath([]string{"target-arrowhead", "shape"}, uq("none"))
	root.edge(splitID(from), splitID(labelID), nil, markGoroutine(noArrow, e))

	root.edge(splitID(labelID), splitID(to), nil, markGoroutine(newD2Map(), e))
}

func addEdgeInline(root *d2Map, from, to string, e collect.Edge, label string) {
	attrs := newD2Map()
	if e.Cond != "" {
		attrs.setPath([]string{d2Style, "bold"}, uq("true"))
	}

	attrs.setPath([]string{d2Style, d2TextTransform}, uq("none"))
	markGoroutine(attrs, e)
	root.edge(splitID(from), splitID(to), blockStr("txt", label), attrs)
}

const goroutineStrokeDash = "3"

func markGoroutine(attrs *d2Map, e collect.Edge) *d2Map {
	if e.Goroutine {
		attrs.setPath([]string{d2Style, "stroke-dash"}, uq(goroutineStrokeDash))
	}

	return attrs
}

const (
	labelGlyphPx = 10
	labelGapPx   = 16
)

func labelNodeWidth(label string) int {
	longest := 0
	for line := range strings.SplitSeq(label, "\n") {
		longest = max(longest, len(line))
	}

	return longest*labelGlyphPx + labelGapPx
}

func labelNodeHeight(label string) int {
	lines := 1 + strings.Count(label, "\n")

	return labelLineBoxPx + int(float64(lines-1)*labelLineHeight) + labelVPadPx
}

const (
	labelLineBoxPx = 24
	labelVPadPx    = 12
)

func edgeLabelNodeID(from, to string) string {
	lane := from[:strings.LastIndex(from, ".")]
	fromNode := from[strings.LastIndex(from, ".")+1:]
	toNode := to[strings.LastIndex(to, ".")+1:]

	return lane + ".lbl_" + fromNode + "_" + toNode
}

func splitID(id string) []string {
	return strings.Split(id, ".")
}

func srcPrefix(outDir string) string {
	depth := len(strings.Split(filepath.ToSlash(filepath.Clean(outDir)), "/"))

	return strings.Repeat("../", depth)
}

func genComment(outDir string) string {
	return fmt.Sprintf("<!-- Code-generated by pantograph. DO NOT EDIT. Run `go generate ./%s`. -->\n", outDir)
}

//go:embed domain.md.tmpl index.md.tmpl
var tmplFS embed.FS

var pageTmpl = template.Must(template.ParseFS(tmplFS, "domain.md.tmpl", "index.md.tmpl")) //nolint:gochecknoglobals // parsed once at init

func mustExecute(name string, data any) string {
	var b strings.Builder

	if err := pageTmpl.ExecuteTemplate(&b, name, data); err != nil {
		panic(fmt.Errorf("execute template %s: %w", name, err))
	}

	return b.String()
}

type domainStep struct {
	Label, Qual, Pos, Link string
}

type domainPage struct {
	Gen, ID, Note, Members, SVGFile, D2Src string
	Width                                  int
	Steps                                  []domainStep
}

func renderMarkdown(f *collect.Flow, svgFile string, width int, d2src, outDir string) string {
	prefix := srcPrefix(outDir)

	steps := make([]domainStep, len(f.Nodes))
	for i, n := range f.Nodes {
		steps[i] = domainStep{
			Label: collect.FuncLabel(n.Qual),
			Qual:  n.Qual,
			Pos:   n.Pos,
			Link:  prefix + posLink(n.Pos),
		}
	}

	return mustExecute("domain.md.tmpl", domainPage{
		Gen:     genComment(outDir),
		ID:      f.ID,
		Note:    f.Note,
		Members: joinCode(f.Members),
		SVGFile: svgFile,
		D2Src:   d2src,
		Width:   width,
		Steps:   steps,
	})
}

func joinCode(ids []string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = "`" + id + "`"
	}

	return strings.Join(parts, ", ")
}

type indexEntry struct {
	ID, Note string
}

type indexPage struct {
	Gen     string
	Entries []indexEntry
}

func renderIndex(ids []string, flows map[string]*collect.Flow, outDir string) string {
	entries := make([]indexEntry, len(ids))
	for i, id := range ids {
		entries[i] = indexEntry{ID: id, Note: flows[id].Note}
	}

	return mustExecute("index.md.tmpl", indexPage{Gen: genComment(outDir), Entries: entries})
}
