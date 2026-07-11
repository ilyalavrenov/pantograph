package collect

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ilyalavrenov/pantograph/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNodeDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line    string
		want    *Node
		wantErr string
	}{
		{
			name: "not a directive",
			line: "// just a normal doc comment",
			want: nil, wantErr: "<skip>",
		},
		{
			name: "flow-id only",
			line: `//pantograph:drain`,
			want: &Node{Flow: "drain"},
		},
		{
			name: "space prefix tolerated",
			line: `// pantograph:drain`,
			want: &Node{Flow: "drain"},
		},
		{
			name: "kind bareword accepted (validated against config, not at parse)",
			line: `//pantograph:drain kind=store`,
			want: &Node{Flow: "drain", Kind: "store"},
		},
		{
			name: "handoff-from",
			line: `//pantograph:drain handoff-from=job-finalize`,
			want: &Node{Flow: "drain", HandoffFrom: []string{"job-finalize"}},
		},
		{
			name: "handoff-to",
			line: `//pantograph:drain handoff-to=confirm-halt`,
			want: &Node{Flow: "drain", HandoffTo: []string{"confirm-halt"}},
		},
		{
			name: "handoff-to is repeatable",
			line: `//pantograph:drain handoff-to=confirm-halt handoff-to=halt-check`,
			want: &Node{Flow: "drain", HandoffTo: []string{"confirm-halt", "halt-check"}},
		},
		{
			name: "cond on a handoff endpoint",
			line: `//pantograph:drain handoff-from=confirm-halt cond="won lock"`,
			want: &Node{Flow: "drain", HandoffFrom: []string{"confirm-halt"},
				HandoffLabels: map[string]EndpointLabel{"confirm-halt": {Cond: "won lock"}}},
		},
		{
			name: "note on a handoff endpoint",
			line: `//pantograph:drain handoff-from=h note="async via callback"`,
			want: &Node{Flow: "drain", HandoffFrom: []string{"h"},
				HandoffLabels: map[string]EndpointLabel{"h": {Note: "async via callback"}}},
		},
		{
			name: "quoted cond preserves spaces and equals, kind may follow",
			line: `//pantograph:drain handoff-from=h cond="ratio>=0.5 and fresh" kind=decision`,
			want: &Node{Flow: "drain", HandoffFrom: []string{"h"}, Kind: "decision",
				HandoffLabels: map[string]EndpointLabel{"h": {Cond: "ratio>=0.5 and fresh"}}},
		},
		{
			name: "cond and note both quoted on one directive",
			line: `//pantograph:drain handoff-from=h cond="won" note="async via callback"`,
			want: &Node{Flow: "drain", HandoffFrom: []string{"h"},
				HandoffLabels: map[string]EndpointLabel{"h": {Cond: "won", Note: "async via callback"}}},
		},
		{
			name: "two handoffs each with its own note",
			line: `//pantograph:architecture handoff-from=a note="signals" handoff-from=b note="persist"`,
			want: &Node{Flow: "architecture", HandoffFrom: []string{"a", "b"},
				HandoffLabels: map[string]EndpointLabel{"a": {Note: "signals"}, "b": {Note: "persist"}}},
		},
		{
			name: "cond binds to the later -from when a -to precedes it",
			line: `//pantograph:drain handoff-to=job-finalize handoff-from=confirm-halt cond="won lock"`,
			want: &Node{Flow: "drain", HandoffTo: []string{"job-finalize"}, HandoffFrom: []string{"confirm-halt"},
				HandoffLabels: map[string]EndpointLabel{"confirm-halt": {Cond: "won lock"}}},
		},
		{
			name:    "cond after handoff-to is an error (labels an incoming edge)",
			line:    `//pantograph:drain handoff-to=h cond="incoming"`,
			wantErr: "labels an incoming edge",
		},
		{
			name:    "cond before its handoff-from is an error (labels no edge)",
			line:    `//pantograph:drain cond="early" handoff-from=h`,
			wantErr: "labels no edge here",
		},
		{
			name:    "unquoted cond is an error",
			line:    `//pantograph:drain handoff-from=h cond=won`,
			wantErr: "cond= needs a quoted value",
		},
		{
			name:    "quoted bareword key is an error",
			line:    `//pantograph:drain kind="decision"`,
			wantErr: "kind= takes a bareword value, not a quoted one",
		},
		{
			name:    "cond on a NON-handoff node is an error",
			line:    `//pantograph:drain cond="orphan"`,
			wantErr: "labels no edge here",
		},
		{
			name:    "-> arrow surfaces a migration error",
			line:    `//pantograph:drain -> "b"`,
			wantErr: "edges are derived; remove -> arrows",
		},
		{
			name:    "unknown key",
			line:    `//pantograph:drain color=red`,
			wantErr: `unknown key "color"`,
		},
		{
			name:    "bareword without = is rejected",
			line:    `//pantograph:drain entry`,
			wantErr: `unknown token "entry"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseNodeDirective(tc.line)

			switch {
			case tc.wantErr == "<skip>":
				require.ErrorIs(t, err, errNotDirective)

				return
			case tc.wantErr != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseCallSiteDirective(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		line     string
		wantID   string
		wantCond string
		wantNote string
		wantErr  string
	}{
		{
			name:    "not a directive",
			line:    "// ordinary comment",
			wantErr: "<skip>",
		},
		{
			name:     "cond only",
			line:     `//pantograph:f cond="won lock"`,
			wantID:   "f",
			wantCond: "won lock",
		},
		{
			name:     "note only",
			line:     `//pantograph:f note="async via goroutine"`,
			wantID:   "f",
			wantNote: "async via goroutine",
		},
		{
			name:     "cond and note both quoted",
			line:     `//pantograph:f cond="won" note="async via goroutine"`,
			wantID:   "f",
			wantCond: "won",
			wantNote: "async via goroutine",
		},
		{
			name:     "quoted value preserves equals and spaces",
			line:     `//pantograph:f note="retries x=3 times"`,
			wantID:   "f",
			wantNote: "retries x=3 times",
		},
		{
			name:    "unquoted note is an error",
			line:    `//pantograph:f note=async`,
			wantErr: "note= needs a quoted value",
		},
		{
			name:    "a node key is rejected here",
			line:    `//pantograph:f kind=store`,
			wantErr: "call-site directive allows only cond=/note=",
		},
		{
			name:    "a bareword is rejected",
			line:    `//pantograph:f entry`,
			wantErr: "call-site directive allows only cond=/note=",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			id, cond, note, err := parseCallSiteDirective(tc.line)

			switch {
			case tc.wantErr == "<skip>":
				require.ErrorIs(t, err, errNotDirective)

				return
			case tc.wantErr != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantID, id)
			assert.Equal(t, tc.wantCond, cond)
			assert.Equal(t, tc.wantNote, note)
		})
	}
}

func TestScanFuncRejectsSameFlowTwice(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "x.go", `package p
//pantograph:f kind=process
//pantograph:f kind=store
func (c *C) dup() {}
`, parser.ParseComments)
	require.NoError(t, err)

	fn, ok := file.Decls[0].(*ast.FuncDecl)
	require.True(t, ok)

	_, err = scanFunc(fset, fn, "p", "", map[string]*Flow{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tagged into flow "f" twice`)
}

func TestRelPosIsCWDIndependent(t *testing.T) {
	t.Parallel()

	got := relPos(token.Position{Filename: "/repo/pkg/worker/x.go", Line: 42}, "/repo")
	assert.Equal(t, "pkg/worker/x.go:42", got)
}

func TestValidateKindsAgainstConfig(t *testing.T) {
	t.Parallel()

	flows := map[string]*Flow{"f": {ID: "f", Nodes: []*Node{
		{Flow: "f", Qual: "p.A", Pos: "x.go:1", Kind: "store"},
		{Flow: "f", Qual: "p.B", Pos: "x.go:2", Kind: "bogus"},
		{Flow: "f", Qual: "p.C", Pos: "x.go:3"},
	}}}

	cfg := &config.Config{Kinds: map[string]string{"store": "cylinder"}}

	err := validateKinds(flows, cfg)
	require.Error(t, err, "an undeclared kind must fail")
	assert.Contains(t, err.Error(), `"bogus"`)
	assert.NotContains(t, err.Error(), `"store"`, "a declared kind passes")
	assert.NotContains(t, err.Error(), "x.go:3", "an omitted kind is allowed")
}

func TestCollectParsesDocCommentsAndProducesModuleRelativePaths(t *testing.T) {
	t.Parallel()

	flows := collectFixture(t, "collectbasic")
	require.NotEmpty(t, flows, "directives must be parsed from doc comments (ParseComments)")

	basic := flows["basic"]
	require.NotNil(t, basic, "the basic flow must be discovered")

	assert.Contains(t, nodeQuals(basic), "collectbasic.Entry")
	assert.Contains(t, nodeQuals(basic), "collectbasic.Step")

	_, ok := edgeIn(basic, "collectbasic.Entry", "collectbasic.Step")
	assert.True(t, ok, "collect must derive the Entry→Step edge from the call graph")

	for id, f := range flows {
		for _, n := range f.Nodes {
			assert.True(t, strings.HasPrefix(n.Pos, "internal/collect/testdata/"),
				"flow %q node %q pos %q must be repo-root-relative", id, n.Qual, n.Pos)
			assert.False(t, filepath.IsAbs(n.Pos),
				"flow %q node %q pos %q must not be absolute", id, n.Qual, n.Pos)
		}
	}
}
