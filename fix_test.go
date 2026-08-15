package boolname

// White-box tests for the fix's scope contracts. Each is about how FAR a
// rename may reach: too narrow leaves the code not compiling, too wide rewrites
// a symbol that merely shares a name. The harness below is shared by every
// white-box test in the package: it builds a one-file package in memory, runs
// the analyzer over it, and applies what the analyzer proposed.

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
