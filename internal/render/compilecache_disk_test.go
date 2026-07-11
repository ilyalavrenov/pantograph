package render

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskCacheWarmMatchesCold(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := renderFlow(smallFlow("a"), map[string]bool{}, testShapes)

	cold, err := newDiskCompileCache(dir).compile(src)
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "cold compile did not populate cache dir")

	warm, err := newDiskCompileCache(dir).compile(src) // fresh instance, same disk
	require.NoError(t, err)

	assert.Equal(t, string(cold), string(warm), "warm compile differs from cold")
}

func TestCacheRootHonorsEnvOverride(t *testing.T) {
	want := t.TempDir()
	t.Setenv(cacheEnvVar, want)

	assert.Equal(t, want, cacheRoot(), "%s override should win over the default", cacheEnvVar)
}
