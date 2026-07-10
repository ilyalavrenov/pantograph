package render

import (
	"regexp"
	"slices"
	"strings"

	"github.com/ilyalavrenov/pantograph/internal/collect"
	"oss.terrastruct.com/d2/d2renderers/d2fonts"
	"oss.terrastruct.com/d2/lib/textmeasure"
)

const labelClear = chipTextInset + labelChipStrokePx + 8

type labelBox struct {
	x, y, w, h float64
	text       string
}

func (b labelBox) overlaps(o labelBox) bool {
	return b.x-labelClear < o.x+o.w+labelClear &&
		o.x-labelClear < b.x+b.w+labelClear &&
		b.y-labelClear < o.y+o.h+labelClear &&
		o.y-labelClear < b.y+b.h+labelClear
}

const chipInflate = chipTextInset + labelChipStrokePx

func (b labelBox) overlapsNode(nodes []labelBox) bool {
	left, right := b.x-chipInflate, b.x+b.w+chipInflate
	top, bot := b.y-chipInflate, b.y+b.h+chipInflate

	return slices.ContainsFunc(nodes, func(o labelBox) bool {
		return left < o.x+o.w && o.x < right && top < o.y+o.h && o.y < bot
	})
}

func (b labelBox) edgeBleedsThrough(segs []segment) bool {
	cx := b.x + b.w/2
	left, right := b.x-chipTextInset, b.x+b.w+chipTextInset

	for _, s := range segs {
		if s.x < left || s.x > right {
			continue
		}

		if s.yBot <= b.y || s.yTop >= b.y+b.h {
			continue
		}

		if s.x < cx-ownEdgeTol || s.x > cx+ownEdgeTol {
			return true
		}
	}

	return false
}

const ownEdgeTol = 24.0

type laneRect struct{ x, y, w, h float64 }

func (b labelBox) crossesLaneBorder(lanes []laneRect) bool {
	left, right := b.x-chipTextInset, b.x+b.w+chipTextInset
	top, bot := b.y, b.y+b.h

	for _, l := range lanes {
		if bot <= l.y || top >= l.y+l.h {
			continue
		}

		for _, bx := range []float64{l.x, l.x + l.w} {
			if bx > left && bx < right {
				return true
			}
		}
	}

	return false
}

func laneBorderRects(svg []byte) []laneRect {
	var out []laneRect

	for _, m := range laneFillRect.FindAllSubmatch(svg, -1) {
		out = append(out, laneRect{x: parseF(m[1]), y: parseF(m[2]), w: parseF(m[3]), h: parseF(m[4])})
	}

	return out
}

var laneFillRect = regexp.MustCompile(
	`<rect x="([\d.]+)" y="([\d.]+)" width="([\d.]+)" height="([\d.]+)"[^>]*fill="` + laneFill + `"`)

var inlineLabelGroup = regexp.MustCompile(
	`<g transform="translate\(([\d.]+) ([\d.]+)\)" class="light-code">((?:[^<]*<text\b[^>]*>[^<]*</text>)+)</g>`)

var inlineLineText = regexp.MustCompile(`<text [^>]*>([^<]*)</text>`)

func inlineLabelBoxes(svg []byte, ruler *textmeasure.Ruler) []labelBox {
	font := d2fonts.SourceCodePro.Font(edgeLabelFontPx, d2fonts.FONT_STYLE_REGULAR)

	var out []labelBox

	for _, m := range inlineLabelGroup.FindAllSubmatch(svg, -1) {
		x, y := parseF(m[1]), parseF(m[2])

		var (
			maxW    float64
			lineHpx float64
			nLines  int
			text    strings.Builder
		)

		for _, l := range inlineLineText.FindAllStringSubmatch(string(m[3]), -1) {
			decoded := decodeSVGText(l[1])

			w, h := ruler.Measure(font, decoded)
			maxW = max(maxW, float64(w))
			lineHpx = max(lineHpx, float64(h))

			if nLines > 0 {
				text.WriteString("\n")
			}

			text.WriteString(decoded)
			nLines++
		}

		if nLines == 0 {
			continue
		}

		height := float64(nLines-1)*labelLineHeight + lineHpx

		out = append(out, labelBox{x: x, y: y, w: maxW, h: height, text: text.String()})
	}

	return out
}

var nodeLabelGroup = regexp.MustCompile(
	`<text x="([\d.]+)" y="([\d.]+)"[^>]*class="text-mono-bold[^"]*"[^>]*>([^<]+)</text>`)

func nodeLabelBoxes(svg []byte, ruler *textmeasure.Ruler) []labelBox {
	font := d2fonts.SourceCodePro.Font(edgeLabelFontPx, d2fonts.FONT_STYLE_BOLD)

	var out []labelBox

	for _, m := range nodeLabelGroup.FindAllSubmatch(svg, -1) {
		cx, baseline := parseF(m[1]), parseF(m[2])
		text := decodeSVGText(string(m[3]))

		w, h := ruler.Measure(font, text)

		out = append(out, labelBox{
			x: cx - float64(w)/2,
			y: baseline - labelAscentPx,
			w: float64(w),
			h: float64(h),
		})
	}

	return out
}

func collidingEdgeKeys(svg []byte, f *collect.Flow, ruler *textmeasure.Ruler) map[string]bool {
	boxes := inlineLabelBoxes(svg, ruler)
	if len(boxes) == 0 {
		return nil
	}

	segs := verticalSegments(svg)
	lanes := laneBorderRects(svg)
	nodeBoxes := nodeLabelBoxes(svg, ruler)
	colliding := make([]bool, len(boxes))

	for i := range boxes {
		if boxes[i].edgeBleedsThrough(segs) || boxes[i].crossesLaneBorder(lanes) || boxes[i].overlapsNode(nodeBoxes) {
			colliding[i] = true
		}

		for j := i + 1; j < len(boxes); j++ {
			if boxes[i].overlaps(boxes[j]) {
				colliding[i] = true
				colliding[j] = true
			}
		}
	}

	wantText := make(map[string]bool)

	for i, c := range colliding {
		if c {
			wantText[boxes[i].text] = true
		}
	}

	keys := make(map[string]bool)

	for _, e := range f.Edges {
		label := edgeLabel(e)
		if label == "" {
			continue
		}

		if wantText[decodeLabelText(label)] {
			keys[useNodeKey(e.From, e.To)] = true
		}
	}

	return keys
}

func decodeLabelText(label string) string {
	var b strings.Builder

	for i, line := range strings.Split(label, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}

		b.WriteString(decodeSVGText(line))
	}

	return b.String()
}
