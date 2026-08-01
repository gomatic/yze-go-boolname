package boolname

// White-box tests for the fix's scope contracts. Each is about how FAR a
// rename may reach: too narrow leaves the code not compiling, too wide rewrites
// a symbol that merely shares a name.

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
)

// checked type-checks src and returns a pass carrying its syntax and types,
// which is what every helper under test takes.
func checked(t *testing.T, src string) (*analysis.Pass, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "p.go", src, parser.ParseComments)
	require.NoError(t, err)

	info := &types.Info{
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{},
	}
	conf := types.Config{Importer: importer.Default()}
	pkg, err := conf.Check("example.test/p", fset, []*ast.File{file}, info)
	require.NoError(t, err)

	return &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}, file
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
	pass, file := checked(t, `package p
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
		assert.Equal(t, tc.want, isBoolean(pass, paramOf(t, file, tc.fn)), "isBoolean(%s)", tc.fn)
	}
}

// TestSweepScopeKeepsAFuncLitOutOfItsEnclosingDoc names sweepScope's claim. The
// comment sweep rewrites prose inside the returned range, so a func literal
// that reached its ENCLOSING function's doc comment would rewrite documentation
// describing a different symbol that happens to share the name.
func TestSweepScopeKeepsAFuncLitOutOfItsEnclosingDoc(t *testing.T) {
	t.Parallel()
	_, file := checked(t, `package p
// outer documents ready.
func outer(ready bool) {
	f := func(ready bool) {}
	_ = f
}
`)
	decl := file.Decls[0].(*ast.FuncDecl)

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
	pass, file := checked(t, `package p
func plain(ready bool) {}
`)
	ident := paramOf(t, file, "plain")

	assert.Same(t, file, fileOf(pass, ident.Pos()))
	assert.Same(t, file, fileOf(pass, file.FileStart), "the first byte is inside the file")
	assert.Same(t, file, fileOf(pass, file.FileEnd-1), "and so is the last")
}

// TestFixesForWithholdsAFixWhenTheNameIsNotRewritable names fixesFor's claims.
// A diagnostic with no fix is the analyzer saying it sees the problem but
// cannot rewrite it safely, and each case below is a way rewriting would break
// the code: an exported name has references this pass cannot see, and a
// proposed name that already exists in scope would collide with a different
// symbol.
func TestFixesForWithholdsAFixWhenTheNameIsNotRewritable(t *testing.T) {
	t.Parallel()
	pass, file := checked(t, `package p
func exported(Ready bool) bool { return Ready }
func collide(ready bool) bool { var isReady = ready; return isReady }
func fine(ready bool) bool { return ready }
`)

	assert.Empty(t, fixesFor(pass, paramOf(t, file, "exported"), fixable(true)),
		"an exported-looking name is outside the heuristic's lowercase domain")
	assert.Empty(t, fixesFor(pass, paramOf(t, file, "collide"), fixable(true)),
		"the proposed name is already taken in an enclosing or nested scope")
	assert.Empty(t, fixesFor(pass, paramOf(t, file, "fine"), fixable(false)),
		"a name marked unfixable carries no fix however clean it looks")
	assert.NotEmpty(t, fixesFor(pass, paramOf(t, file, "fine"), fixable(true)),
		"a rewritable name must actually get a fix, or the assertions above prove nothing")
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
