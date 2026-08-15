package boolname

// White-box tests for reconciling the proposals against each other: which of
// two candidates wanting one identifier keeps it, and what the rewrite must
// never do to earn that answer.

import (
	"go/ast"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReconciledLetsTheFirstClaimantKeepTheName names reconciled's claim. Two
// candidates proposing one identifier is the defect this pass exists to catch,
// and which of them keeps it must be settled the same way on every run. It is
// settled by the order the traversal reaches them, which is NOT source order —
// an enclosing signature is visited before one nested inside it, so the last
// case below has the textually later name winning.
//
// A nested signature yields only when renaming both would CAPTURE a reference,
// which is a fact about the fixture and not about nesting. Withdrawing on
// nesting alone is permanent: the withheld name is never renamed by any later
// run either, because by then the enclosing name is declared and collides
// refuses to shadow it. So the nested cases below run in both directions —
// captured and not — and an earlier revision of this test asserted the empty
// fix for all of them.
func TestReconciledLetsTheFirstClaimantKeepTheName(t *testing.T) {
	t.Parallel()
	src := analyzed(t, `package p
func siblings(ix bool, ıx bool) bool { return ix && ıx }
func nested(ix bool) bool {
	inner := func(ıx bool) bool { return ıx }
	return inner(ix)
}
func captured(ix bool) bool {
	inner := func(ıx bool) bool { return ıx && !ix }
	return inner(ix)
}
func shadowed(ready bool) bool {
	inner := func(ready bool) bool { return ready }
	return inner(ready)
}
func elsewhere(ix bool) bool { return ix }
func inverted(g func(ix bool), ıx bool) bool { return ıx }
`)
	require.Len(t, src.diagnostics, 11, "every ill-named boolean is reported, fix or no fix")

	for i, tc := range []struct{ want, why string }{
		{want: "rename ix to isIx", why: "the first claimant in a signature keeps the name"},
		{want: "", why: "its sibling must yield, or the signature declares isIx twice"},
		{want: "rename ix to isIx", why: "a different signature is a separate contest"},
		{want: "rename ıx to isIx", why: "the nested signature reads nothing from the enclosing one, so shadowing it captures nothing"},
		{want: "rename ix to isIx", why: "the enclosing name is renamed whether or not the nested one may be"},
		{want: "", why: "the nested body reads the enclosing ix, which renaming both would silently rebind to the nested one"},
		{want: "rename ready to isReady", why: "an enclosing name the nested signature already shadows"},
		{want: "rename ready to isReady", why: "and the nested one, which shadows it before and after, so nothing there ever resolved to the enclosing object"},
		{want: "rename ix to isIx", why: "an unrelated signature never contends"},
		{want: "rename ıx to isIx", why: "the enclosing signature is visited first, though its name comes second in the line"},
		{want: "rename ix to isIx", why: "a bodyless nested signature has no reference anywhere, so nothing there can be captured"},
	} {
		fixes := src.diagnostics[i].SuggestedFixes
		if tc.want == "" {
			assert.Empty(t, fixes, tc.why)
			continue
		}
		require.Len(t, fixes, 1, tc.why)
		assert.Equal(t, tc.want, fixes[0].Message, tc.why)
	}
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

// TestNoFixChangesWhatAnIdentifierResolvesTo names the promise no compile check
// can express, and the reason a nested contender may ever be withdrawn at all.
// Capturing a reference produces source that BUILDS: rename both halves of
// `func f(ix bool) { g := func(ıx bool) bool { return ıx && !ix } }` to isIx and
// the literal becomes `func(isIx bool) bool { return isIx && !isIx }`, which
// compiles, vets clean and is constant false. A silent rebinding is strictly
// worse than a redeclaration error, because nothing reports it.
func TestNoFixChangesWhatAnIdentifierResolvesTo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, src, why string }{
		{
			name: "capturedByTheNestedSignature",
			src: `package p
func held(ix bool) bool {
	inner := func(ıx bool) bool { return ıx && !ix }
	return inner(ix)
}
`,
			why: "the nested body reads the enclosing ix, so one identifier for both would rebind it to the literal's own parameter",
		},
		{
			name: "alreadyShadowed",
			src: `package p
func shadowed(ready bool) bool {
	inner := func(ready bool) bool { return ready }
	return inner(ready)
}
`,
			why: "the nested declaration shadows the enclosing one before and after, so renaming both moves nothing",
		},
		{
			name: "nestedSignatureReadingNothingOutside",
			src: `package p
func apart(ix bool) bool {
	inner := func(ıx bool) bool { return ıx }
	return inner(ix)
}
`,
			why: "nothing inside the literal resolved to the enclosing ix, so there is nothing for the shared name to capture",
		},
		{
			name: "bodylessNestedSignature",
			src: `package p
func fb(g func(sx bool), ſx bool) bool { _ = g; return ſx }
`,
			why: "a func type's parameter name has no references at all",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, resolution(t, tc.src), resolution(t, analyzed(t, tc.src).applied()), tc.why)
		})
	}
}
