package boolname

// White-box tests for the capture primitive and for the pair of names one pass
// introduces. Every contract here is about a PAIR of scopes and the references
// between them: whether shadowing would rebind a read, and whether the answer
// depends on anything but that read. What collides asks of a name against the
// source as it stands is collide_test.go's.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCapturesSeesOnlyAReadInsideTheNestedScope names captures' claim, which is
// the whole discriminator and which neither the compiler nor a golden file can
// check — a capture BUILDS. What is pinned here is that it is DIRECTIONAL: only
// the enclosing declaration is the one shadowed, so the same pair handed the
// other way round captures nothing.
//
// An earlier revision of this comment claimed a second thing — that counting
// declarations as well as reads would make every pair capture — and that was
// false for this code and provable in one line: the only ident it would add is
// the outer's own declaration, which lies outside every strictly-nested extent.
// The reads-not-declarations narrowing is readsWithin's, and it is stated and
// cased there, where the one region that can hold a symbol's own declaration
// is reachable.
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

// TestReadsWithinCountsReadsAndNotDeclarations names readsWithin's claim, which
// is the primitive every shadowing decision in this file is built out of. A
// shadow rebinds the READS sitting under it; a declaration is what the shadow
// is measured against, not something it moves. The third case is the one that
// separates the two: ax is declared in the scope it is asked about and read
// nowhere at all, so there is nothing under a shadow there to rebind, and
// counting its declaration would say otherwise.
func TestReadsWithinCountsReadsAndNotDeclarations(t *testing.T) {
	t.Parallel()
	src, declarations := contenders(t)
	only := func(param string) candidate { return declarations(param)[0] }

	assert.True(t, readsWithin(src.pass, only("ix").obj, only("iy").obj.Parent()),
		"the nested body reads ix, and that read is exactly what a shadow there would rebind")
	assert.False(t, readsWithin(src.pass, only("px").obj, only("py").obj.Parent()),
		"the nested body reads nothing of px, so a shadow over it moves nothing")
	assert.False(t, readsWithin(src.pass, only("ax").obj, only("ax").obj.Parent()),
		"ax is declared in that scope and read nowhere, and a declaration is never a read")
}
