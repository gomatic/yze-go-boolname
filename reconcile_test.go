package boolname

// White-box tests for reconciling the proposals against each other. Every
// contract here is about a PAIR: whether two candidates wanting one identifier
// may both have it, and what the rewrite must never do to earn that answer.

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

// TestCapturesSeesOnlyAReadInsideTheNestedScope names captures' claim, which is
// the whole discriminator and which neither the compiler nor a golden file can
// check — a capture BUILDS. Two claims are pinned here. It counts READS, not
// every identifier the rename rewrites: an enclosing declaration always sits
// inside its own scope, so counting declarations would make every pair capture.
// And it is DIRECTIONAL: only the enclosing declaration is the one shadowed, so
// the same pair handed the other way round captures nothing.
func TestCapturesSeesOnlyAReadInsideTheNestedScope(t *testing.T) {
	t.Parallel()
	src, declarations := contenders(t)
	only := func(param string) candidate { return declarations(param)[0] }
	shadowed := declarations("ready")
	require.Len(t, shadowed, 2, "the shadowing fixture must declare ready twice")

	for _, tc := range []struct {
		outer, inner candidate
		why          string
		want         bool
	}{
		{outer: only("ix"), inner: only("iy"), want: true, why: "the nested body reads the enclosing ix, so one identifier would rebind that read"},
		{outer: only("px"), inner: only("py"), want: false, why: "the nested body reads nothing from the enclosing signature"},
		{outer: shadowed[0], inner: shadowed[1], want: false, why: "the nested declaration already shadows the enclosing one, so nothing there reads it"},
		{outer: only("tx"), inner: only("sx"), want: false, why: "a bodyless nested signature has no body for a read to sit in"},
		{outer: only("distinct"), inner: only("ix"), want: false, why: "unrelated signatures do not nest at all"},
	} {
		t.Run(tc.outer.name.Name+"-"+tc.inner.name.Name, func(t *testing.T) {
			assert.Equal(t, tc.want, captures(src.pass, tc.outer, tc.inner), tc.why)
			assert.False(t, captures(src.pass, tc.inner, tc.outer),
				"the nested scope encloses nothing, so it can never be the one doing the capturing: %s", tc.why)
		})
	}
}

// TestContendsAnswersTheSameBothWays names contends' claim. It decides whether
// two proposals may both be taken, and it is handed the pair in whatever order
// the traversal produced them. A predicate that answered differently for
// (outer, inner) than for (inner, outer) would let a colliding proposal through
// whenever the traversal happened to reach the nested signature first — and
// would strand a safe one whenever it did not. The first case is the one
// captures cannot answer: a redeclaration in one scope is illegal whether or
// not either name is ever read, so nothing about references decides it.
func TestContendsAnswersTheSameBothWays(t *testing.T) {
	t.Parallel()
	src, declarations := contenders(t)
	only := func(param string) candidate { return declarations(param)[0] }
	shadowed := declarations("ready")
	require.Len(t, shadowed, 2, "the shadowing fixture must declare ready twice")

	for _, tc := range []struct {
		a, b candidate
		why  string
		want bool
	}{
		{a: only("ax"), b: only("bx"), want: true, why: "one scope cannot hold the identifier twice, however little either name is read"},
		{a: only("ix"), b: only("iy"), want: true, why: "the nested body reads the enclosing ix, so one identifier would rebind it"},
		{a: only("px"), b: only("py"), want: false, why: "the nested body reads nothing from the enclosing signature"},
		{a: shadowed[0], b: shadowed[1], want: false, why: "the nested declaration already shadows the enclosing one"},
		{a: only("sx"), b: only("tx"), want: false, why: "a bodyless nested signature has no reference to capture"},
		{a: only("distinct"), b: only("ix"), want: false, why: "sibling signatures never contend"},
	} {
		t.Run(tc.a.name.Name+"-"+tc.b.name.Name, func(t *testing.T) {
			assert.Equal(t, tc.want, contends(src.pass, tc.a, tc.b), tc.why)
			assert.Equal(t, tc.want, contends(src.pass, tc.b, tc.a), "answered the other way round: %s", tc.why)
		})
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
