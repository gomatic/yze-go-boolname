package boolname

// The harness every white-box test in this package is built on, and nothing
// else: it compiles a one-file package in memory, runs the analyzer over it,
// applies what the analyzer proposed, and reads back the two things no golden
// comparison can see — which object each identifier resolves to, and which
// candidates a predicate is being asked about. It declares no test of its own
// because it asserts nothing; it is the instrument the assertions are made
// with, which is also why it is not named for a source file: there is no
// harness.go for it to be the unit test of.

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// source is a single-file package parsed and type-checked in memory, together
// with whatever the analyzer reported over it.
type source struct {
	fset        *token.FileSet
	file        *ast.File
	pass        *analysis.Pass
	err         error
	text        string
	diagnostics []analysis.Diagnostic
}

// compile parses and type-checks text as a one-file package. err is nil exactly
// when the Go type checker accepts the text, so a rewritten source that fails
// to build fails here, with the same message the compiler gives.
func compile(text string) *source {
	built := &source{text: text, fset: token.NewFileSet()}
	file, err := parser.ParseFile(built.fset, "p.go", text, parser.ParseComments)
	if err != nil {
		built.err = err
		return built
	}
	built.file = file

	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("example.test/p", built.fset, []*ast.File{file}, info)
	built.err = err
	built.pass = &analysis.Pass{
		Fset:      built.fset,
		Files:     []*ast.File{file},
		Pkg:       pkg,
		TypesInfo: info,
		ResultOf:  map[*analysis.Analyzer]any{inspect.Analyzer: inspector.New([]*ast.File{file})},
		Report:    func(d analysis.Diagnostic) { built.diagnostics = append(built.diagnostics, d) },
	}
	return built
}

// checked is compile plus the requirement that the text itself builds, which
// every fixture must before anything is asserted about rewriting it.
func checked(t *testing.T, text string) *source {
	t.Helper()
	built := compile(text)
	require.NoError(t, built.err)
	return built
}

// analyzed is checked plus a run of the analyzer, so diagnostics is populated.
func analyzed(t *testing.T, text string) *source {
	t.Helper()
	built := checked(t, text)
	_, err := run(built.pass)
	require.NoError(t, err)
	return built
}

// applied returns the text with every edit of every suggested fix spliced in,
// which is what the `-fix` driver does. Edits are applied from the end so an
// earlier offset is never disturbed by a later replacement.
func (s *source) applied() string {
	var edits []analysis.TextEdit
	for _, diagnostic := range s.diagnostics {
		for _, fix := range diagnostic.SuggestedFixes {
			edits = append(edits, fix.TextEdits...)
		}
	}
	slices.SortFunc(edits, func(a, b analysis.TextEdit) int { return int(b.Pos - a.Pos) })
	text := s.text
	for _, edit := range edits {
		at, to := s.fset.Position(edit.Pos).Offset, s.fset.Position(edit.End).Offset
		text = text[:at] + string(edit.NewText) + text[to:]
	}
	return text
}

// proposals returns the name each fix would introduce, read back out of the
// rewritten declaration rather than out of the fix's own message.
func (s *source) proposals() []identName {
	var proposed []identName
	for _, diagnostic := range s.diagnostics {
		for _, fix := range diagnostic.SuggestedFixes {
			proposed = append(proposed, identName(fix.TextEdits[0].NewText))
		}
	}
	return proposed
}

// paramOf returns the first parameter identifier of the named function.
func paramOf(t *testing.T, file *ast.File, fn string) *ast.Ident {
	t.Helper()
	for _, decl := range file.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if ok && d.Name.Name == fn {
			return d.Type.Params.List[0].Names[0]
		}
	}
	t.Fatalf("no func %s", fn)
	return nil
}

// resolution returns, for every identifier in text in source order, the
// position in that same list of the identifier declaring what it resolves to,
// or -1 for a declaration and for anything declared outside the file. It is
// invariant under renaming — spellings and columns move, the shape does not —
// so comparing it across a rewrite asks the one question the type checker will
// not: does everything still mean what it meant?
func resolution(t *testing.T, text string) []int {
	t.Helper()
	built := checked(t, text)

	var idents []*ast.Ident
	ast.Inspect(built.file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			idents = append(idents, id)
		}
		return true
	})
	declaredAt := map[types.Object]int{}
	for at, id := range idents {
		if obj := built.pass.TypesInfo.Defs[id]; obj != nil {
			declaredAt[obj] = at
		}
	}
	resolved := make([]int, len(idents))
	for at, id := range idents {
		declared, isKnown := declaredAt[built.pass.TypesInfo.Uses[id]]
		if !isKnown {
			declared = -1
		}
		resolved[at] = declared
	}
	return resolved
}

// contenders builds the fixture both predicates below are judged on and returns
// a lookup from a parameter name to its declarations, in source order.
func contenders(t *testing.T) (*source, func(string) []candidate) {
	t.Helper()
	src := checked(t, `package p
func siblings(ax bool, bx bool) {}
func captured(ix bool) bool {
	inner := func(iy bool) bool { return iy && ix }
	return inner(ix)
}
func free(px bool) bool {
	inner := func(py bool) bool { return py }
	return inner(px)
}
func shadowing(ready bool) bool {
	inner := func(ready bool) bool { return ready }
	return inner(ready)
}
func bodyless(g func(sx bool), tx bool) bool { return tx }
func apart(distinct bool) bool { return distinct }
`)
	return src, func(param string) []candidate {
		var found []candidate
		ast.Inspect(src.file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && id.Name == param && src.pass.TypesInfo.Defs[id] != nil {
				found = append(found, candidate{name: id, obj: src.pass.TypesInfo.Defs[id]})
			}
			return true
		})
		require.NotEmpty(t, found, "no declaration of %s", param)
		return found
	}
}

// shape is one source a collision contract is judged on: whether the pass
// leaves it SETTLED — nothing still reported — which is the only observable
// difference between a rename withheld and a rename taken, since a withheld one
// is reported again on every run forever.
type shape struct {
	name, src, why string
	isSettled      bool
}

// settles runs each shape through the pass and asserts the three things a
// rename must satisfy: the rewrite builds, no identifier resolves anywhere new,
// and whatever is still reported afterwards is what the analyzer will report
// for good.
func settles(t *testing.T, shapes []shape) {
	t.Helper()
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			once := analyzed(t, tc.src).applied()

			assert.NoError(t, compile(once).err, "the rewritten source must build: %s", tc.why)
			assert.Equal(t, resolution(t, tc.src), resolution(t, once), "%s", tc.why)
			assert.Equal(t, tc.isSettled, len(analyzed(t, once).diagnostics) == 0,
				"whatever is still reported here is reported on every run forever: %s", tc.why)
		})
	}
}
