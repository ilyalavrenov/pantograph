package collect

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

type label struct {
	cond string
	note string
	flow string
}

type edgeKey struct{ from, to string }

func deriveEdges(fset *token.FileSet, flows map[string]*Flow, st *scanState) error {
	derived := map[string]map[edgeKey]Edge{}

	for _, tf := range st.funcs {
		if err := deriveFromFunc(fset, tf, st, derived); err != nil {
			return err
		}
	}

	appendDerived(flows, derived)

	return nil
}

func deriveFromFunc(fset *token.FileSet, tf taggedFunc, st *scanState, derived map[string]map[edgeKey]Edge) error {
	labels, err := callSiteLabels(fset, tf, st)
	if err != nil {
		return err
	}

	matched := map[*ast.CallExpr]bool{}

	asyncCalls := goStmtCalls(tf.fn.Body)

	var walkErr error

	ast.Inspect(tf.fn.Body, func(n ast.Node) bool {
		if walkErr != nil {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		callee := resolveCallee(tf.info, call)
		if callee == nil {
			return true
		}

		for _, src := range tf.nodes {
			targets, terr := matchTargets(callee, src, st)
			if terr != nil {
				walkErr = terr

				return false
			}

			emitTargets(call, src, targets, labels, matched, derived, asyncCalls[call])
		}

		return true
	})

	if walkErr != nil {
		return walkErr
	}

	return checkUnmatchedLabels(fset, labels, matched)
}

func emitTargets(
	call *ast.CallExpr,
	src *Node,
	targets []*Node,
	labels map[*ast.CallExpr]label,
	matched map[*ast.CallExpr]bool,
	derived map[string]map[edgeKey]Edge,
	async bool,
) {
	for _, tgt := range targets {
		if tgt.Flow != src.Flow || tgt.Qual == src.Qual {
			continue
		}

		key := edgeKey{from: src.Qual, to: tgt.Qual}
		e := Edge{From: src.Qual, To: tgt.Qual, Goroutine: async}

		if lb, ok := labels[call]; ok && lb.flow == src.Flow {
			e.Cond = lb.cond
			e.Note = lb.note
			matched[call] = true
		}

		if derived[src.Flow] == nil {
			derived[src.Flow] = map[edgeKey]Edge{}
		}

		if prev, ok := derived[src.Flow][key]; ok {
			if prev.Cond == "" && prev.Note == "" && (e.Cond != "" || e.Note != "") {
				e.Goroutine = e.Goroutine || prev.Goroutine
				derived[src.Flow][key] = e
			} else if e.Goroutine && !prev.Goroutine {
				prev.Goroutine = true
				derived[src.Flow][key] = prev
			}

			continue
		}

		derived[src.Flow][key] = e
	}
}

func goStmtCalls(body *ast.BlockStmt) map[*ast.CallExpr]bool {
	async := map[*ast.CallExpr]bool{}

	ast.Inspect(body, func(n ast.Node) bool {
		gs, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}

		async[gs.Call] = true

		if lit, ok := ast.Unparen(gs.Call.Fun).(*ast.FuncLit); ok {
			ast.Inspect(lit.Body, func(m ast.Node) bool {
				if call, ok := m.(*ast.CallExpr); ok {
					async[call] = true
				}

				return true
			})
		}

		return true
	})

	return async
}

func appendDerived(flows map[string]*Flow, derived map[string]map[edgeKey]Edge) {
	for id, m := range derived {
		f := flows[id]
		if f == nil {
			continue
		}

		edges := make([]Edge, 0, len(m))
		for _, e := range m {
			edges = append(edges, e)
		}

		sortEdges(edges)

		f.Edges = append(f.Edges, edges...)
	}
}

func resolveCallee(info *types.Info, call *ast.CallExpr) *types.Func {
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		fn, _ := info.Uses[fun].(*types.Func) //nolint:errcheck // nil on non-Func (var, builtin) is the intended skip signal

		return fn
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[fun]; ok {
			fn, _ := sel.Obj().(*types.Func) //nolint:errcheck // nil on non-Func is the intended skip signal

			return fn
		}

		fn, _ := info.Uses[fun.Sel].(*types.Func) //nolint:errcheck // nil on non-Func is the intended skip signal

		return fn
	default:
		return nil
	}
}

func matchTargets(callee *types.Func, src *Node, st *scanState) ([]*Node, error) {
	if nodes, ok := st.tagged[callee]; ok {
		return nodes, nil
	}

	if !isInterfaceMethod(callee) {
		return nil, nil
	}

	var inFlow []*Node

	for _, n := range st.taggedByName[callee.Name()] {
		if n.Flow == src.Flow {
			inFlow = append(inFlow, n)
		}
	}

	if len(inFlow) > 1 {
		return nil, fmt.Errorf("pantograph:%s: interface method %q matches multiple tagged funcs (%s); ambiguous edge",
			src.Flow, callee.Name(), quals(inFlow))
	}

	return inFlow, nil
}

func isInterfaceMethod(obj *types.Func) bool {
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return false
	}

	recv := sig.Recv()

	return recv != nil && types.IsInterface(recv.Type())
}

func quals(nodes []*Node) string {
	qs := make([]string, len(nodes))
	for i, n := range nodes {
		qs[i] = n.Qual
	}

	sort.Strings(qs)

	return strings.Join(qs, ", ")
}

func callSiteLabels(fset *token.FileSet, tf taggedFunc, st *scanState) (map[*ast.CallExpr]label, error) {
	out := map[*ast.CallExpr]label{}

	cmap := ast.NewCommentMap(fset, tf.file, tf.file.Comments)

	for node, groups := range cmap {
		if node.Pos() < tf.fn.Body.Pos() || node.End() > tf.fn.Body.End() {
			continue
		}

		if err := scanCommentGroups(fset, node, groups, tf, st, out); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func scanCommentGroups(
	fset *token.FileSet,
	node ast.Node,
	groups []*ast.CommentGroup,
	tf taggedFunc,
	st *scanState,
	out map[*ast.CallExpr]label,
) error {
	for _, grp := range groups {
		for _, c := range grp.List {
			flowID, cond, note, err := parseCallSiteDirective(c.Text)
			if errors.Is(err, errNotDirective) {
				continue
			}

			if err != nil {
				return fmt.Errorf("%s: %w", relPosFset(fset, c.Pos()), err)
			}

			call := firstTaggedCall(node, tf, st, flowID)
			if call == nil {
				return fmt.Errorf("%s: pantograph:%s: call-site directive matches no tagged call on its statement",
					relPosFset(fset, c.Pos()), flowID)
			}

			out[call] = label{cond: cond, note: note, flow: flowID}
		}
	}

	return nil
}

func firstTaggedCall(stmt ast.Node, tf taggedFunc, st *scanState, flowID string) *ast.CallExpr {
	var found *ast.CallExpr

	ast.Inspect(stmt, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		callee := resolveCallee(tf.info, call)
		if callee != nil && calleeTargetsFlow(callee, flowID, st) {
			found = call

			return false
		}

		return true
	})

	return found
}

func calleeTargetsFlow(callee *types.Func, flowID string, st *scanState) bool {
	for _, n := range st.tagged[callee] {
		if n.Flow == flowID {
			return true
		}
	}

	if isInterfaceMethod(callee) {
		for _, n := range st.taggedByName[callee.Name()] {
			if n.Flow == flowID {
				return true
			}
		}
	}

	return false
}

func checkUnmatchedLabels(fset *token.FileSet, labels map[*ast.CallExpr]label, matched map[*ast.CallExpr]bool) error {
	for call, lb := range labels {
		if matched[call] {
			continue
		}

		return fmt.Errorf("%s: pantograph:%s: call-site directive matched no derived edge "+
			"(the enclosing func is not a node in flow %q)",
			relPosFset(fset, call.Pos()), lb.flow, lb.flow)
	}

	return nil
}

func relPosFset(fset *token.FileSet, pos token.Pos) string {
	return fset.Position(pos).String()
}

type handoffEnds struct {
	from *Node
	to   *Node
}

func pairHandoffs(flows map[string]*Flow) error {
	groups, err := groupHandoffs(flows)
	if err != nil {
		return err
	}

	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}

	sort.Strings(names)

	touched := map[string]bool{}

	for _, name := range names {
		ends := groups[name]

		if err := validateHandoffEnds(name, ends); err != nil {
			return err
		}

		applyHandoff(flows, name, ends)
		touched[ends.from.Flow] = true
	}

	for id := range touched {
		sortEdges(flows[id].Edges)
	}

	return nil
}

func groupHandoffs(flows map[string]*Flow) (map[string]*handoffEnds, error) {
	groups := map[string]*handoffEnds{}

	ends := func(name string) *handoffEnds {
		e := groups[name]
		if e == nil {
			e = &handoffEnds{}
			groups[name] = e
		}

		return e
	}

	for _, f := range flows {
		for _, n := range f.Nodes {
			for _, name := range n.HandoffFrom {
				e := ends(name)
				if e.from != nil {
					return nil, fmt.Errorf("handoff %q has multiple from-endpoints (%s and %s)", name, e.from.Qual, n.Qual)
				}

				e.from = n
			}

			for _, name := range n.HandoffTo {
				e := ends(name)
				if e.to != nil {
					return nil, fmt.Errorf("handoff %q has multiple to-endpoints (%s and %s)", name, e.to.Qual, n.Qual)
				}

				e.to = n
			}
		}
	}

	return groups, nil
}

func validateHandoffEnds(name string, ends *handoffEnds) error {
	if ends.from == nil {
		return fmt.Errorf("handoff %q has only one endpoint (a to-endpoint at %s, no matching from)", name, ends.to.Qual)
	}

	if ends.to == nil {
		return fmt.Errorf("handoff %q has only one endpoint (a from-endpoint at %s, no matching to)", name, ends.from.Qual)
	}

	if ends.from.Flow != ends.to.Flow {
		return fmt.Errorf("handoff %q crosses flows (%s vs %s)", name, ends.from.Flow, ends.to.Flow)
	}

	return nil
}

func applyHandoff(flows map[string]*Flow, name string, ends *handoffEnds) {
	cond, note := handoffLabel(name, ends)
	f := flows[ends.from.Flow]

	for i := range f.Edges {
		e := &f.Edges[i]
		if e.From != ends.from.Qual || e.To != ends.to.Qual {
			continue
		}

		e.Handoff = true

		if e.Cond == "" {
			e.Cond = cond
		}

		if e.Note == "" {
			e.Note = note
		}

		return
	}

	f.Edges = append(f.Edges, Edge{
		From:    ends.from.Qual,
		To:      ends.to.Qual,
		Cond:    cond,
		Note:    note,
		Handoff: true,
	})
}

func handoffLabel(name string, ends *handoffEnds) (string, string) {
	lbl := ends.from.HandoffLabels[name]

	return lbl.Cond, lbl.Note
}

func sortEdges(edges []Edge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}

		return edges[i].To < edges[j].To
	})
}
