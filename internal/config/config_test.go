package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func write(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600))

	return dir
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	t.Parallel()

	c, err := Load(t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, "diamond", c.Kinds["decision"])
	assert.True(t, c.KnownKind("event"))
	assert.Empty(t, c.DomainDecls())
}

func TestLoadKindsAndDomains(t *testing.T) {
	t.Parallel()

	c, err := Load(write(t, `
kinds:
  gateway: hexagon
  store:   cylinder
domains:
  pipeline:
    flows: [ingest, report]
    note: the pipeline
  runtime: [exit, reconcile]
`))
	require.NoError(t, err)

	assert.Equal(t, "hexagon", c.Kinds["gateway"])
	assert.True(t, c.KnownKind("gateway"))
	assert.False(t, c.KnownKind("bogus"), "an undeclared kind is unknown")
	assert.True(t, c.KnownKind(""), "an omitted kind is always valid")

	decls := c.DomainDecls()
	require.Len(t, decls, 2)
	assert.Equal(t, "pipeline", decls[0].Domain, "domains sort by name")
	assert.Equal(t, []string{"ingest", "report"}, decls[0].Flows)
	assert.Equal(t, "the pipeline", decls[0].Note)
	assert.Equal(t, []string{"exit", "reconcile"}, decls[1].Flows, "a bare list decodes as flows")
}

func TestLoadRejectsUnknownShape(t *testing.T) {
	t.Parallel()

	_, err := Load(write(t, "kinds:\n  x: blob\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown shape")
}

func TestLoadRejectsEmptyDomain(t *testing.T) {
	t.Parallel()

	_, err := Load(write(t, "domains:\n  d:\n    flows: []\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no flows")
}

func TestLoadRejectsUnknownField(t *testing.T) {
	t.Parallel()

	_, err := Load(write(t, "kinds:\n  x: page\nbogus: 1\n"))
	require.Error(t, err)
}
