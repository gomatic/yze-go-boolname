package boolname

// White-box tests for refusing a proposal against the source AS IT STANDS. Each
// case is judged on whether the pass leaves the shape SETTLED, because that is
// the only observable difference between a rename taken and a rename withheld:
// a withheld one is reported again on every run, forever, and no later run
// finds the source any different.

import (
	"testing"
)

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
			name:      "nameAboveReadUnderALaterLocal",
			isSettled: false,
			why:       "the literal reads the package-level isVerbose; the local of that name is declared BELOW the literal and shadows nothing inside it, so it is the package one whose reads decide",
			src: `package p

var isVerbose = true

func f() bool {
	g := func(verbose bool) bool { return verbose && isVerbose }
	isVerbose := false
	_ = isVerbose
	return g(true)
}
`,
		},
		{
			name:      "typeAboveReadUnderALaterLocal",
			isSettled: false,
			why:       "the shadowed symbol need not be a variable and the later declaration need not be one either: a package-level TYPE read inside the literal, under a const declared below it",
			src: `package p

type isBig struct{ N int }

func f() int {
	g := func(big bool) int {
		v := isBig{N: 1}
		if big {
			return v.N
		}
		return 0
	}
	const isBig = 7
	_ = isBig
	return g(true)
}
`,
		},
		{
			name:      "unreadNameAboveUnderALaterLocal",
			isSettled: true,
			why:       "the same later local with nothing of that name read inside the literal, so a declaration existing above is still not on its own a refusal",
			src: `package p

var isVerbose = true

func f() bool {
	g := func(verbose bool) bool { return verbose }
	isVerbose := false
	_ = isVerbose
	return g(true)
}
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
