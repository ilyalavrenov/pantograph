package render

import (
	"regexp"
	"strconv"
)

type pt struct{ x, y float64 }

type seg2d struct{ a, b pt }

type edgePath []seg2d

var connectionStrokePath = regexp.MustCompile(`<path d="([^"]*)"[^>]*class="connection stroke-`)

var pathCoord = regexp.MustCompile(`-?\d+\.?\d*`)

func parseConnectionPaths(svg []byte) []edgePath {
	var edges []edgePath

	for _, m := range connectionStrokePath.FindAllSubmatch(svg, -1) {
		nums := pathCoord.FindAllString(string(m[1]), -1)

		pts := make([]pt, 0, len(nums)/2)
		for i := 0; i+1 < len(nums); i += 2 {
			x, _ := strconv.ParseFloat(nums[i], 64)   //nolint:errcheck // matched by float regex, always valid
			y, _ := strconv.ParseFloat(nums[i+1], 64) //nolint:errcheck // matched by float regex, always valid
			pts = append(pts, pt{x, y})
		}

		if len(pts) < 2 {
			continue
		}

		segs := make(edgePath, 0, len(pts)-1)
		for i := 0; i+1 < len(pts); i++ {
			segs = append(segs, seg2d{pts[i], pts[i+1]})
		}

		edges = append(edges, segs)
	}

	return edges
}

func countEdgeCrossings(edges []edgePath) int {
	crossings := 0

	for i := range edges {
		for j := i + 1; j < len(edges); j++ {
			if edgesCross(edges[i], edges[j]) {
				crossings++
			}
		}
	}

	return crossings
}

func edgesCross(a, b edgePath) bool {
	for _, sa := range a {
		for _, sb := range b {
			if segmentsProperlyIntersect(sa, sb) {
				return true
			}
		}
	}

	return false
}

const endpointEps = 2.0

func segmentsProperlyIntersect(sa, sb seg2d) bool {
	if sharesEndpoint(sa, sb) {
		return false
	}

	d1 := orient(sb.a, sb.b, sa.a)
	d2 := orient(sb.a, sb.b, sa.b)
	d3 := orient(sa.a, sa.b, sb.a)
	d4 := orient(sa.a, sa.b, sb.b)

	return (d1 > 0) != (d2 > 0) && (d3 > 0) != (d4 > 0)
}

func sharesEndpoint(sa, sb seg2d) bool {
	return near(sa.a, sb.a) || near(sa.a, sb.b) || near(sa.b, sb.a) || near(sa.b, sb.b)
}

func near(p, q pt) bool {
	dx := p.x - q.x
	dy := p.y - q.y

	return dx > -endpointEps && dx < endpointEps && dy > -endpointEps && dy < endpointEps
}

func orient(a, b, c pt) float64 {
	return (b.x-a.x)*(c.y-a.y) - (b.y-a.y)*(c.x-a.x)
}
