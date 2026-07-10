package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func chainLanes() []laneRect {
	return []laneRect{
		{x: 31, y: 617, w: 269, h: 166},
		{x: 176, y: 12, w: 510, h: 424},
		{x: 320, y: 617, w: 507, h: 218},
	}
}

func TestCrossesLaneBorderFlagsStraddlingChip(t *testing.T) {
	t.Parallel()

	b := labelBox{x: 65 + chipTextInset, y: 314, w: 388 - 65 - 2*chipTextInset, h: 32}

	require.True(t, b.crossesLaneBorder(chainLanes()),
		"a chip straddling lane A's left border within its band must be flagged")
}

func TestCrossesLaneBorderIgnoresNonCrossingChips(t *testing.T) {
	t.Parallel()

	contained := labelBox{x: 350, y: 200, w: 200, h: 32}
	require.False(t, contained.crossesLaneBorder(chainLanes()),
		"a chip wholly inside its lane must NOT be flagged")

	otherBand := labelBox{x: 290, y: 100, w: 70, h: 32}
	require.False(t, otherBand.crossesLaneBorder(chainLanes()),
		"a border in a different y-band must NOT flag the chip")

	atEdge := labelBox{x: 200, y: 100, w: 100, h: 32}
	rightEdge := 200 + 100 + float64(chipTextInset)
	singleLane := []laneRect{{x: 0, y: 0, w: rightEdge, h: 400}}

	require.False(t, atEdge.crossesLaneBorder(singleLane),
		"a border exactly at the chip edge grazes, does not slice — must not flag")
}
