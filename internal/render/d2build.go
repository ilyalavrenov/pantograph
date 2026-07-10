package render

import (
	"fmt"
	"strings"

	"oss.terrastruct.com/d2/d2ast"
	"oss.terrastruct.com/d2/d2format"
)

const (
	d2Style         = "style"
	d2TextTransform = "text-transform"
)

//nolint:ireturn // d2ast.Value is the d2 library's value interface; returning it is intentional
func uq(s string) d2ast.Value { return d2ast.FlatUnquotedString(s) }

//nolint:ireturn // d2ast.Value is the d2 library's value interface; returning it is intentional
func strVal(s string) d2ast.Value { return d2ast.RawString(s, false) }

//nolint:ireturn // d2ast.Value is the d2 library's value interface; returning it is intentional
func blockStr(tag, value string) d2ast.Value {
	return &d2ast.BlockString{Tag: tag, Value: value}
}

type d2Map struct {
	items []d2ast.MapNodeBox
}

func newD2Map() *d2Map { return &d2Map{} }

func (m *d2Map) add(node d2ast.MapNodeBox) {
	m.items = append(m.items, node)
}

func (m *d2Map) addKey(k *d2ast.Key) {
	m.add(d2ast.MakeMapNodeBox(k))
}

func (m *d2Map) comment(value string) {
	m.add(d2ast.MakeMapNodeBox(&d2ast.Comment{Value: value}))
}

func (m *d2Map) set(key string, val d2ast.Value) {
	m.setPath([]string{key}, val)
}

func (m *d2Map) setPath(path []string, val d2ast.Value) {
	m.addKey(&d2ast.Key{
		Key:   d2ast.MakeKeyPath(path),
		Value: d2ast.MakeValueBox(val),
	})
}

func (m *d2Map) child(key string, child *d2Map) {
	m.addKey(&d2ast.Key{
		Key:   d2ast.MakeKeyPath([]string{key}),
		Value: d2ast.MakeValueBox(child.toMap()),
	})
}

func (m *d2Map) labeledChild(key string, label d2ast.Value, child *d2Map) {
	m.labeledChildPath([]string{key}, label, child)
}

func (m *d2Map) labeledChildPath(path []string, label d2ast.Value, child *d2Map) {
	m.addKey(&d2ast.Key{
		Key:     d2ast.MakeKeyPath(path),
		Primary: d2ast.MakeValueBox(label).ScalarBox(),
		Value:   d2ast.MakeValueBox(child.toMap()),
	})
}

func (m *d2Map) edge(src, dst []string, label d2ast.Value, attrs *d2Map) {
	k := &d2ast.Key{
		Edges: []*d2ast.Edge{{
			Src:      d2ast.MakeKeyPath(src),
			DstArrow: ">",
			Dst:      d2ast.MakeKeyPath(dst),
		}},
	}

	if label != nil {
		k.Primary = d2ast.MakeValueBox(label).ScalarBox()
	}

	if attrs != nil {
		k.Value = d2ast.MakeValueBox(attrs.toMap())
	}

	m.addKey(k)
}

func (m *d2Map) toMap() *d2ast.Map {
	return &d2ast.Map{Nodes: m.items}
}

func (m *d2Map) format() string {
	return d2format.Format(m.finish())
}

func (m *d2Map) finish() *d2ast.Map {
	mp := m.toMap()

	line := 0
	stampNodes(mp.Nodes, &line)

	mp.Range = d2ast.MakeRange(fmt.Sprintf(",0:0:0-%d:0:0", line+1))

	return mp
}

func stampNodes(nodes []d2ast.MapNodeBox, line *int) {
	for _, nb := range nodes {
		switch {
		case nb.MapKey != nil:
			stampKey(nb.MapKey, line)
		case nb.Comment != nil:
			start := *line + 1
			*line += strings.Count(nb.Comment.Value, "\n") + 1
			nb.Comment.Range = lineRange(start, *line)
		}
	}
}

func stampKey(k *d2ast.Key, line *int) {
	*line++
	start := *line

	if k.Value.Map != nil {
		stampNodes(k.Value.Map.Nodes, line)
		k.Value.Map.Range = d2ast.MakeRange(fmt.Sprintf(",%d:1:0-%d:0:0", start, *line+1))
	}

	k.Range = lineRange(start, *line)
}

func lineRange(start, end int) d2ast.Range {
	return d2ast.MakeRange(fmt.Sprintf(",%d:0:0-%d:0:0", start, end))
}
