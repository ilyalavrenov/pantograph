package render

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShapeScorePrefersColumnFitOverWideAspect(t *testing.T) {
	t.Parallel()

	fitDown := shapeScore(838, 865)
	wideRight := shapeScore(1422, 535)

	assert.Less(t, fitDown, wideRight,
		"a column-fitting layout must beat a wider-but-better-aspect one that overflows GitHub's column")
}

func TestShapeScoreDegenerateHeight(t *testing.T) {
	t.Parallel()

	assert.True(t, math.IsInf(shapeScore(800, 0), 1), "zero height must score +Inf")
	assert.True(t, math.IsInf(shapeScore(0, 500), 1), "zero width must score +Inf")
}
