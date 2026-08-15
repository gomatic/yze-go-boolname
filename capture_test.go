package boolname

// White-box tests for what makes two symbols unable to share one identifier.
// Every contract here is about a PAIR of scopes and the references between
// them: whether shadowing would rebind a read, and whether the answer depends
// on anything but that read.

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

// TestShadowsAReadAboveRefusesOnlyWhereTheShadowedNameIsRead names the first
// half of collides. A symbol declared outside the candidate's scope is shadowed
// by the rename throughout that scope, so the refusal is owed exactly where
// something in there reads it — and nowhere else, because ordinary shadowing is
// legal Go that moves nothing while the refusal it earns is permanent: the name
// is reported on every run forever and no later run offers a fix.
func TestShadowsAReadAboveRefusesOnlyWhereTheShadowedNameIsRead(t *testing.T) {
	t.Parallel()
	settles(t, []shape{
		{
			name:      "sameScope",
			isSettled: false,
			why:       "one scope cannot hold isReady twice, and no fact about reads makes that legal",
			src: `package p
func f(ready bool) bool { var isReady = ready; return isReady }
`,
		},
		{
			name:      "unreadNameAbove",
			isSettled: true,
			why:       "nothing inside the literal reads the enclosing isReady, so shadowing it rebinds nothing",
			src: `package p
func f(isReady bool) bool { inner := func(ready bool) bool { return ready }; return inner(isReady) }
`,
		},
		{
			name:      "readNameAbove",
			isSettled: false,
			why:       "the literal reads the enclosing isReady, which the rename would rebind to the literal's own parameter",
			src: `package p
func f(isReady bool) bool { inner := func(ready bool) bool { return ready && isReady }; return inner(isReady) }
`,
		},
		{
			name:      "unreadPackageName",
			isSettled: true,
			why:       "a package-level isReady no body reads is shadowed by the rename and read by nothing the shadow covers",
			src: `package p
var isReady = false
func f(ready bool) bool { return ready }
`,
		},
		{
			name:      "readPackageName",
			isSettled: false,
			why:       "the body reads the package-level isReady, so the rename would rebind it to the parameter",
			src: `package p
var isReady = false
func f(ready bool) bool { return ready && isReady }
`,
		},
		{
			name:      "innermostNameAbove",
			isSettled: false,
			why:       "a local isReady stands between the literal and the package one, and it is the local the literal's read resolves to, so it is the local whose reads decide",
			src: `package p
var isReady = false
func f(seen bool) bool {
	isReady := true
	inner := func(ready bool) bool { return ready && isReady }
	return inner(seen)
}
`,
		},
	})
}

// TestShadowedByAReadBelowRefusesOnlyWhereTheRenameLandsUnderAShadow names the
// other half. Here the shadowing declaration is already in the source and the
// rename walks under it, so what is captured is a read of the CANDIDATE sitting
// inside that nested scope. The walk covers the whole subtree, and the cases
// below sit the declaration one, two and three scopes down, because a bound
// anywhere in that recursion answers the shallowest case identically and
// silently rewrites everything deeper.
func TestShadowedByAReadBelowRefusesOnlyWhereTheRenameLandsUnderAShadow(t *testing.T) {
	t.Parallel()
	settles(t, []shape{
		{
			name:      "unreadNameBelow",
			isSettled: true,
			why:       "the literal declares isReady already and reads nothing of the enclosing signature, so the rename lands under a shadow that covers nothing",
			src: `package p
func f(ready bool) bool { inner := func(isReady bool) bool { return isReady }; return inner(ready) }
`,
		},
		{
			name:      "readNameBelow",
			isSettled: false,
			why:       "the literal reads the enclosing ready, which its own isReady captures the moment the rename lands",
			src: `package p
func f(ready bool) bool { inner := func(isReady bool) bool { return isReady && ready }; return inner(ready) }
`,
		},
		{
			name:      "unreadLocalBelow",
			isSettled: true,
			why:       "a short variable declaration in a block shadows the renamed parameter there and reads nothing of it, which is the shape the corpus refused for four commits",
			src: `package p
func f(ready bool) bool {
	if ready {
		isReady := true
		return isReady
	}
	return ready
}
`,
		},
		{
			name:      "readLocalBelow",
			isSettled: false,
			why:       "the same block reads the parameter after declaring isReady, so the rename would put that read under the local one",
			src: `package p
func f(ready bool) bool {
	if ready {
		isReady := true
		return isReady && ready
	}
	return ready
}
`,
		},
		{
			name:      "readNameTwoScopesBelow",
			isSettled: false,
			why:       "a bare block puts the shadowing declaration two scopes down, where a walk over the immediate children alone would not find it",
			src: `package p
func f(ready bool) bool {
	{
		inner := func(isReady bool) bool { return isReady && ready }
		return inner(ready)
	}
}
`,
		},
		{
			name:      "unreadNameTwoScopesBelow",
			isSettled: true,
			why:       "the same distance in the direction where nothing is captured, so the distance is not itself the refusal",
			src: `package p
func f(ready bool) bool {
	{
		inner := func(isReady bool) bool { return isReady }
		return inner(ready)
	}
}
`,
		},
		{
			name:      "readNameThreeScopesBelow",
			isSettled: false,
			why:       "an if opens a scope for its header as well as its body, so the shadowing declaration sits deeper again",
			src: `package p
func f(ready bool) bool {
	if ready {
		inner := func(isReady bool) bool { return isReady && ready }
		return inner(ready)
	}
	return ready
}
`,
		},
	})
}
