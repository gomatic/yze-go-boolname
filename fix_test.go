package boolname

// White-box tests for the fix's scope contracts. Each is about how FAR a
// rename may reach: too narrow leaves the code not compiling, too wide rewrites
// a symbol that merely shares a name. The harness below is shared by every
// white-box test in the package: it builds a one-file package in memory, runs
// the analyzer over it, and applies what the analyzer proposed.

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

// TestIsBooleanUnwrapsExactlyOnePointerLevel names isBoolean's claim. The
// analyzer only renames boolean-typed names, so this predicate decides whether
// a symbol is touched at all. One pointer level is deliberate — a *bool
// parameter reads like a bool at the call site — while **bool and a ~bool type
// parameter are out of scope, and treating them as booleans would rename
// symbols the heuristic was never validated against.
func TestIsBooleanUnwrapsExactlyOnePointerLevel(t *testing.T) {
	t.Parallel()
	src := checked(t, `package p
type Flag bool
func plain(ready bool) {}
func pointer(ready *bool) {}
func named(ready Flag) {}
func namedPointer(ready *Flag) {}
func double(ready **bool) {}
func notBool(ready string) {}
func generic[T ~bool](ready T) {}
`)
	for _, tc := range []struct {
		fn   string
		want bool
	}{
		{fn: "plain", want: true},
		{fn: "pointer", want: true},
		{fn: "named", want: true},
		{fn: "namedPointer", want: true},
		{fn: "double", want: false},
		{fn: "notBool", want: false},
		{fn: "generic", want: false},
	} {
		assert.Equal(t, tc.want, isBoolean(src.pass, paramOf(t, src.file, tc.fn)), "isBoolean(%s)", tc.fn)
	}
}

// TestSweepScopeKeepsAFuncLitOutOfItsEnclosingDoc names sweepScope's claim. The
// comment sweep rewrites prose inside the returned range, so a func literal
// that reached its ENCLOSING function's doc comment would rewrite documentation
// describing a different symbol that happens to share the name.
func TestSweepScopeKeepsAFuncLitOutOfItsEnclosingDoc(t *testing.T) {
	t.Parallel()
	src := checked(t, `package p
// outer documents ready.
func outer(ready bool) {
	f := func(ready bool) {}
	_ = f
}
`)
	decl := src.file.Decls[0].(*ast.FuncDecl)

	doc, lo, hi := sweepScope(decl)
	require.NotNil(t, doc, "a func declaration contributes its doc comment")
	assert.Equal(t, decl.Body.Pos(), lo)
	assert.Equal(t, decl.Body.End(), hi)

	var lit *ast.FuncLit
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if l, ok := n.(*ast.FuncLit); ok {
			lit = l
		}
		return true
	})
	require.NotNil(t, lit)

	litDoc, litLo, litHi := sweepScope(lit)
	assert.Nil(t, litDoc, "a func literal must never sweep the enclosing function's doc")
	assert.Equal(t, lit.Pos(), litLo)
	assert.Equal(t, lit.End(), litHi)

	nilDoc, nilLo, nilHi := sweepScope(nil)
	assert.Nil(t, nilDoc)
	assert.Equal(t, token.NoPos, nilLo)
	assert.Equal(t, token.NoPos, nilHi)
}

// TestFileOfFindsTheFileContainingAPosition names fileOf's claim. It indexes
// pass.Files with the result of IndexFunc, so "the lookup always succeeds" is
// not a remark — a miss returns -1 and panics. Every reported ident does come
// from pass.Files, and this pins that the containing file is found by range
// rather than by order.
func TestFileOfFindsTheFileContainingAPosition(t *testing.T) {
	t.Parallel()
	src := checked(t, `package p
func plain(ready bool) {}
`)
	ident := paramOf(t, src.file, "plain")

	assert.Same(t, src.file, fileOf(src.pass, ident.Pos()))
	assert.Same(t, src.file, fileOf(src.pass, src.file.FileStart), "the first byte is inside the file")
	assert.Same(t, src.file, fileOf(src.pass, src.file.FileEnd-1), "and so is the last")
}

// TestIsWordRejectsAMentionInsideALongerIdentifier names isWord's claim. The
// comment sweep replaces the old name wherever it appears as a word; without
// this boundary test, renaming "run" would rewrite "dryRun" and "laundry"
// inside prose, corrupting sentences that had nothing to do with the symbol.
func TestIsWordRejectsAMentionInsideALongerIdentifier(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		text commentText
		why  string
		at   byteOffset
		want bool
	}{
		{text: "run it", at: 0, want: true, why: "start of text is a boundary"},
		{text: "we run it", at: 3, want: true, why: "spaces are boundaries"},
		{text: "the run", at: 4, want: true, why: "end of text is a boundary"},
		{text: "dryRun", at: 3, want: false, why: "preceded by an identifier rune"},
		{text: "runny", at: 0, want: false, why: "followed by an identifier rune, with a clean leading boundary"},
		{text: "a run3", at: 2, want: false, why: "a digit is an identifier rune too"},
		{text: "call run()", at: 5, want: true, why: "punctuation is a boundary"},
		{text: "run", at: 0, want: true, why: "the whole text"},
	} {
		assert.Equal(t, tc.want, isWord(tc.text, tc.at, byteCount(len("run"))),
			"isWord(%q, %d): %s", tc.text, tc.at, tc.why)
	}
}
