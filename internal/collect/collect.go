package collect

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ilyalavrenov/pantograph/internal/config"
	"golang.org/x/tools/go/packages"
)

const defaultPattern = "./..."

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedModule |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps

const discoverMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedModule

type taggedFunc struct {
	fn    *ast.FuncDecl
	info  *types.Info
	pkg   *types.Package
	file  *ast.File
	nodes []*Node
}

type scanState struct {
	tagged       map[types.Object][]*Node
	taggedByName map[string][]*Node
	funcs        []taggedFunc
}

func Collect(pats []string) (map[string]*Flow, []FuncInfo, *config.Config, error) {
	root, err := moduleDir()
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := config.Load(root)
	if err != nil {
		return nil, nil, nil, err
	}

	inv, annotatedPaths, err := discover(pats, root)
	if err != nil {
		return nil, nil, nil, err
	}

	flows, err := deriveFlows(annotatedPaths, root)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := validateKinds(flows, cfg); err != nil {
		return nil, nil, nil, err
	}

	return flows, inv, cfg, nil
}

func validateKinds(flows map[string]*Flow, cfg *config.Config) error {
	var unknown []string

	for id, f := range flows {
		for _, n := range f.Nodes {
			if !cfg.KnownKind(n.Kind) {
				unknown = append(unknown, fmt.Sprintf("%s: flow %q: node %q has unknown kind %q (declare it in %s kinds: or fix the kind=)",
					n.Pos, id, FuncLabel(n.Qual), n.Kind, config.FileName))
			}
		}
	}

	if len(unknown) == 0 {
		return nil
	}

	sort.Strings(unknown)

	return fmt.Errorf("%d unknown kind(s):\n%s", len(unknown), strings.Join(unknown, "\n"))
}

func discover(pats []string, root string) ([]FuncInfo, []string, error) {
	fset := token.NewFileSet()

	pkgs, err := loadPackages(discoverMode, fset, root, pats)
	if err != nil {
		return nil, nil, err
	}

	var (
		inv            []FuncInfo
		annotatedPaths []string
	)

	for _, pkg := range pkgs {
		pkgInv, annotated, err := discoverPackage(fset, pkg, root)
		if err != nil {
			return nil, nil, err
		}

		inv = append(inv, pkgInv...)

		if annotated {
			annotatedPaths = append(annotatedPaths, pkg.PkgPath)
		}
	}

	return inv, annotatedPaths, nil
}

func deriveFlows(annotatedPaths []string, root string) (map[string]*Flow, error) {
	flows := map[string]*Flow{}

	if len(annotatedPaths) == 0 {
		return flows, nil
	}

	fset := token.NewFileSet()

	pkgs, err := loadPackages(loadMode, fset, root, annotatedPaths)
	if err != nil {
		return nil, err
	}

	st := &scanState{
		tagged:       map[types.Object][]*Node{},
		taggedByName: map[string][]*Node{},
	}

	for _, pkg := range pkgs {
		if err := scanPackage(fset, pkg, root, flows, st); err != nil {
			return nil, err
		}
	}

	if err := deriveEdges(fset, flows, st); err != nil {
		return nil, err
	}

	if err := pairHandoffs(flows); err != nil {
		return nil, err
	}

	return flows, nil
}

func loadPackages(mode packages.LoadMode, fset *token.FileSet, root string, pats []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: mode,
		Fset: fset,
		Dir:  root,
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
	}

	pkgs, err := packages.Load(cfg, pats...)
	if err != nil {
		return nil, fmt.Errorf("load packages %v: %w", pats, err)
	}

	if n := packages.PrintErrors(pkgs); n > 0 {
		var first error

		packages.Visit(pkgs, nil, func(p *packages.Package) {
			if first == nil && len(p.Errors) > 0 {
				first = p.Errors[0]
			}
		})

		return nil, fmt.Errorf("loading packages reported %d error(s): %w", n, first)
	}

	return pkgs, nil
}

func moduleDir() (string, error) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedModule}, defaultPattern)
	if err != nil {
		return "", fmt.Errorf("resolve module root: %w", err)
	}

	for _, pkg := range pkgs {
		if pkg.Module != nil && pkg.Module.Dir != "" {
			return pkg.Module.Dir, nil
		}
	}

	return "", errors.New("no enclosing Go module found from the current directory")
}

func OutputRelPath(outDir string) (string, error) {
	root, err := moduleDir()
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(outDir)
	if err != nil {
		return "", fmt.Errorf("resolve output dir: %w", err)
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", fmt.Errorf("output dir relative to module root: %w", err)
	}

	return filepath.ToSlash(rel), nil
}

func discoverPackage(fset *token.FileSet, pkg *packages.Package, root string) ([]FuncInfo, bool, error) {
	pkgPath := relPkgPath(pkg)

	var (
		inv       []FuncInfo
		annotated bool
	)

	discard := map[string]*Flow{}

	for _, file := range pkg.Syntax {
		fileInv, fileAnnotated, err := discoverFile(fset, file, pkg.Name, pkgPath, root, discard)
		if err != nil {
			return nil, false, err
		}

		inv = append(inv, fileInv...)
		annotated = annotated || fileAnnotated
	}

	return inv, annotated, nil
}

func discoverFile(
	fset *token.FileSet, file *ast.File, pkgName, pkgPath, root string, discard map[string]*Flow,
) ([]FuncInfo, bool, error) {
	var (
		inv       []FuncInfo
		annotated bool
	)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		nodes, err := scanFunc(fset, fn, pkgName, root, discard)
		if err != nil {
			return nil, false, err
		}

		if len(nodes) > 0 {
			annotated = true
		}

		if fn.Name.IsExported() {
			inv = append(inv, FuncInfo{
				Qual:      qualName(pkgName, fn),
				PkgPath:   pkgPath,
				Pos:       relPos(fset.Position(fn.Pos()), root),
				Annotated: len(nodes) > 0,
			})
		}
	}

	typeNodes, err := scanTypeDecls(fset, file, pkgName, root, discard)
	if err != nil {
		return nil, false, err
	}

	if len(typeNodes) > 0 {
		annotated = true
	}

	return inv, annotated, nil
}

func scanPackage(fset *token.FileSet, pkg *packages.Package, root string, flows map[string]*Flow, st *scanState) error {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			nodes, err := scanFunc(fset, fn, pkg.Name, root, flows)
			if err != nil {
				return err
			}

			registerTaggedFunc(pkg, file, fn, nodes, st)
		}

		if _, err := scanTypeDecls(fset, file, pkg.Name, root, flows); err != nil {
			return err
		}
	}

	return nil
}

func scanTypeDecls(fset *token.FileSet, file *ast.File, pkg, root string, flows map[string]*Flow) ([]*Node, error) {
	var nodes []*Node

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}

		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			doc := ts.Doc
			if doc == nil && len(gen.Specs) == 1 {
				doc = gen.Doc
			}

			specNodes, err := scanTypeDecl(fset, doc, ts, pkg, root, flows)
			if err != nil {
				return nil, err
			}

			nodes = append(nodes, specNodes...)
		}
	}

	return nodes, nil
}

func registerTaggedFunc(pkg *packages.Package, file *ast.File, fn *ast.FuncDecl, nodes []*Node, st *scanState) {
	if len(nodes) == 0 {
		return
	}

	if obj := funcObject(pkg.TypesInfo, fn); obj != nil {
		st.tagged[obj] = append(st.tagged[obj], nodes...)
		st.taggedByName[obj.Name()] = append(st.taggedByName[obj.Name()], nodes...)
	}

	if fn.Body != nil {
		st.funcs = append(st.funcs, taggedFunc{
			fn:    fn,
			info:  pkg.TypesInfo,
			pkg:   pkg.Types,
			file:  file,
			nodes: nodes,
		})
	}
}

func funcObject(info *types.Info, fn *ast.FuncDecl) *types.Func {
	if info == nil || fn.Name == nil {
		return nil
	}

	obj, _ := info.Defs[fn.Name].(*types.Func) //nolint:errcheck // nil on non-Func is the intended skip signal

	return obj
}

func relPkgPath(pkg *packages.Package) string {
	if pkg.Module == nil || pkg.Module.Path == "" {
		return pkg.PkgPath
	}

	if pkg.PkgPath == pkg.Module.Path {
		return ""
	}

	return strings.TrimPrefix(pkg.PkgPath, pkg.Module.Path+"/")
}

func scanFunc(fset *token.FileSet, fn *ast.FuncDecl, pkg, root string, flows map[string]*Flow) ([]*Node, error) {
	if fn.Doc == nil {
		return nil, nil
	}

	return scanDocNodes(fset, fn.Doc, qualName(pkg, fn), pkg, relPos(fset.Position(fn.Pos()), root), root, flows)
}

func scanTypeDecl(
	fset *token.FileSet, doc *ast.CommentGroup, spec *ast.TypeSpec, pkg, root string, flows map[string]*Flow,
) ([]*Node, error) {
	if doc == nil {
		return nil, nil
	}

	nodes, err := scanDocNodes(fset, doc, qualType(pkg, spec), pkg, relPos(fset.Position(spec.Pos()), root), root, flows)
	if err != nil {
		return nil, err
	}

	for _, n := range nodes {
		if n.Kind == "" {
			n.Kind = "process"
		}
	}

	return nodes, nil
}

func scanDocNodes(
	fset *token.FileSet,
	doc *ast.CommentGroup,
	qual, pkg, declPos, root string,
	flows map[string]*Flow,
) ([]*Node, error) {
	var nodes []*Node

	seenFlows := map[string]bool{}

	for _, c := range doc.List {
		n, derr := parseNodeDirective(c.Text)
		if errors.Is(derr, errNotDirective) {
			continue
		}

		pos := relPos(fset.Position(c.Pos()), root)

		if derr != nil {
			return nil, fmt.Errorf("%s: %w", pos, derr)
		}

		if seenFlows[n.Flow] {
			return nil, fmt.Errorf("%s: declaration tagged into flow %q twice", pos, n.Flow)
		}

		seenFlows[n.Flow] = true

		n.Qual = qual
		n.Lane = pkg
		n.Pos = declPos

		f := flows[n.Flow]
		if f == nil {
			f = &Flow{ID: n.Flow}
			flows[n.Flow] = f
		}

		f.Nodes = append(f.Nodes, n)
		nodes = append(nodes, n)
	}

	return nodes, nil
}

func qualType(pkg string, spec *ast.TypeSpec) string {
	return fmt.Sprintf("%s.%s", pkg, spec.Name.Name)
}

func qualName(pkg string, fn *ast.FuncDecl) string {
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := typeName(fn.Recv.List[0].Type)

		return fmt.Sprintf("%s.%s.%s", pkg, recv, fn.Name.Name)
	}

	return fmt.Sprintf("%s.%s", pkg, fn.Name.Name)
}

func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return typeName(t.X)
	default:
		return "?"
	}
}

func relPos(p token.Position, root string) string {
	file := p.Filename

	if abs, err := filepath.Abs(file); err == nil {
		file = abs
	}

	if rel, err := filepath.Rel(root, file); err == nil && !strings.HasPrefix(rel, "..") {
		file = rel
	}

	return fmt.Sprintf("%s:%d", filepath.ToSlash(file), p.Line)
}
