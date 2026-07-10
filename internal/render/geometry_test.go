package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConnectionPathsExtractsStrokeEdges(t *testing.T) {
	t.Parallel()

	svg := `<svg>
<path d="M 0 0 L 0 10" class="connection stroke-B1" />
<path d="M 5 0 L 5 20 S 5 20 5 20 L 30 20" class="connection stroke-B1" />
<path d="M 1 1 L 2 2" class="connection fill-B1" />
</svg>`

	edges := parseConnectionPaths([]byte(svg))
	require.Len(t, edges, 2, "stroke edges")

	assert.Len(t, edges[0], 1, "edge 0 segments")
	assert.Len(t, edges[1], 4, "edge 1 segments")
}

func TestCountEdgeCrossings(t *testing.T) {
	t.Parallel()

	x1 := edgePath{{pt{0, 0}, pt{10, 10}}}
	x2 := edgePath{{pt{0, 10}, pt{10, 0}}}

	assert.Equal(t, 1, countEdgeCrossings([]edgePath{x1, x2}), "crossing X")

	sharedEndpoint := edgePath{{pt{0, 0}, pt{10, 10}}}
	sharedEndpointOther := edgePath{{pt{0, 0}, pt{-10, 10}}}

	assert.Equal(t, 0, countEdgeCrossings([]edgePath{sharedEndpoint, sharedEndpointOther}), "shared endpoint")

	parallelA := edgePath{{pt{0, 0}, pt{0, 10}}}
	parallelB := edgePath{{pt{5, 0}, pt{5, 10}}}

	assert.Equal(t, 0, countEdgeCrossings([]edgePath{parallelA, parallelB}), "parallel")
}
