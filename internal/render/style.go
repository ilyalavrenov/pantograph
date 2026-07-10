package render

import (
	"sort"
	"strings"

	"github.com/ilyalavrenov/pantograph/internal/collect"
)

const edgeLabelClass = "elabel"

func laneID(lane string) string { return "lane_" + nodeID(lane) }

func laneNodeID(n *collect.Node) string {
	return laneID(n.Lane) + "." + nodeID(collect.FuncLabel(n.Qual))
}

func sortedLanes(nodes []*collect.Node) []string {
	set := map[string]bool{}
	for _, n := range nodes {
		set[n.Lane] = true
	}

	lanes := make([]string, 0, len(set))
	for l := range set {
		lanes = append(lanes, l)
	}

	sort.Strings(lanes)

	return lanes
}

func addNode(lane *d2Map, n *collect.Node, shapes map[string]string) {
	label := collect.FuncLabel(n.Qual)
	id := nodeID(label)

	if shape := shapes[n.Kind]; n.Kind != "" && shape != "" {
		shaped := newD2Map()
		shaped.set("class", uq(n.Kind))
		lane.labeledChild(id, strVal(label), shaped)

		return
	}

	attrs := newD2Map()
	attrs.set("shape", uq("rectangle"))
	attrs.setPath([]string{d2Style, d2TextTransform}, uq("none"))
	lane.labeledChild(id, strVal(label), attrs)
}

const wrapWidth = 48

func wrapLabel(s string) string {
	if len(s) <= wrapWidth {
		return s
	}

	var b strings.Builder

	lineLen := 0

	for i, word := range strings.Fields(s) {
		switch {
		case i == 0:
			b.WriteString(word)
			lineLen = len(word)
		case lineLen+1+len(word) > wrapWidth:
			b.WriteByte('\n')
			b.WriteString(word)
			lineLen = len(word)
		default:
			b.WriteByte(' ')
			b.WriteString(word)
			lineLen += 1 + len(word)
		}
	}

	return b.String()
}

func addClasses(root *d2Map, shapes map[string]string) {
	classes := newD2Map()

	laneStyle := newD2Map()
	laneStyle.set("stroke-dash", uq("3"))
	laneStyle.set(d2TextTransform, uq("none"))

	lane := newD2Map()
	lane.child(d2Style, laneStyle)
	classes.child("lane", lane)

	elabelStyle := newD2Map()
	elabelStyle.set("stroke-width", uq("0"))
	elabelStyle.set(d2TextTransform, uq("none"))

	elabel := newD2Map()
	elabel.set("shape", uq("text"))
	elabel.child(d2Style, elabelStyle)
	classes.child(edgeLabelClass, elabel)

	kinds := make([]string, 0, len(shapes))
	for k := range shapes {
		kinds = append(kinds, k)
	}

	sort.Strings(kinds)

	for _, k := range kinds {
		shape := shapes[k]
		if shape == "" {
			continue
		}

		kindClass := newD2Map()
		kindClass.set("shape", uq(shape))
		kindClass.setPath([]string{d2Style, d2TextTransform}, uq("none"))
		classes.child(k, kindClass)
	}

	root.child("classes", classes)
}
