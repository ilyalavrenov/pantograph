package render

import (
	"testing"

	"github.com/ilyalavrenov/pantograph/internal/collect"
	"github.com/stretchr/testify/require"
)

func BenchmarkCompileSVGRealFlows(b *testing.B) {
	flows, _, _, err := collect.Collect([]string{"./internal/collect/testdata/..."})
	require.NoError(b, err)
	require.NotEmpty(b, flows, "expected //pantograph: annotations in the fixtures")

	b.ReportMetric(float64(len(flows)), "flows/op")
	b.ResetTimer()

	for b.Loop() {
		if _, err := Render(flows, testShapes, "docs/flows"); err != nil {
			b.Fatal(err)
		}
	}
}
