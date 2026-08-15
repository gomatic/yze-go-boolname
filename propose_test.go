package boolname

// White-box tests for what the analyzer proposes for ONE name, against the code
// as it stands. Whether two proposals may coexist is reconcile_test.go's.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProposalForWithholdsWhenTheNameIsNotRewritable names proposalFor's
// claims. An empty proposal is the analyzer saying it sees the problem but
// cannot rewrite it safely, and each case below is a way rewriting would break
// the code or the rule: an exported name has references this pass cannot see, a
// proposed name that already exists in scope would collide with a different
// symbol, and a first rune with no upper case yields a proposal the rule
// reports again the moment it is written.
func TestProposalForWithholdsWhenTheNameIsNotRewritable(t *testing.T) {
	t.Parallel()
	src := checked(t, `package p
func exported(Ready bool) bool { return Ready }
func collide(ready bool) bool { var isReady = ready; return isReady }
func underscore(_verbose bool) bool { return _verbose }
func caseless(有効 bool) bool { return 有効 }
func fine(ready bool) bool { return ready }
`)
	proposalOf := func(fn string, isFixable fixable) identName {
		name := paramOf(t, src.file, fn)
		return proposalFor(name, src.pass.TypesInfo.Defs[name], isFixable)
	}

	assert.Empty(t, proposalOf("exported", fixable(true)),
		"an exported-looking name is outside the heuristic's lowercase domain")
	assert.Empty(t, proposalOf("collide", fixable(true)),
		"the proposed name is already taken in an enclosing or nested scope")
	assert.Empty(t, proposalOf("underscore", fixable(true)),
		"is_verbose has no word boundary after the prefix, so the rule rejects it")
	assert.Empty(t, proposalOf("caseless", fixable(true)),
		"有 has no upper case, so no is-prefixed rename of it can satisfy the rule")
	assert.Empty(t, proposalOf("fine", fixable(false)),
		"a name marked unfixable carries no proposal however clean it looks")
	assert.Equal(t, identName("isReady"), proposalOf("fine", fixable(true)),
		"a rewritable name must actually get its rename, or the assertions above prove nothing")
}

// TestEveryFixCompiles names the analyzer's central promise about its own
// output: source it rewrites still builds. The promise is not discharged by
// checking a proposal against the scope as it stands, because the other names
// the same pass proposes are not in that scope yet — two parameters of one
// signature renamed to the same identifier is "redeclared in this block", and
// no golden comparison can see it.
//
// isSettled is the second half, and it is what compilation cannot express: a
// name the analyzer reports but never rewrites is a fixed point that builds
// perfectly and is reported forever. Every shape whose names CAN be renamed
// leaves no finding behind, so a withdrawal that fires where it need not shows
// up here rather than being mistaken for caution.
func TestEveryFixCompiles(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		src       string
		why       string
		isChanged bool
		isSettled bool
	}{
		{
			name: "collidingSiblings",
			src: `package p
func dotless(ix bool, ıx bool) bool { return ix && ıx }
`,
			why:       "ASCII i and dotless ı (U+0131) upcase to the same I, so only one of the two may take isIx",
			isChanged: true,
			isSettled: false,
		},
		{
			name: "collidingSiblingsNeitherRead",
			src: `package p
func unread(ix bool, ıx bool) {}
`,
			why:       "one scope cannot hold isIx twice whether or not either name is ever read, which is what separates a redeclaration from a shadowing",
			isChanged: true,
			isSettled: false,
		},
		{
			name: "collidingSiblingsAcrossAFold",
			src: `package p
func longs(ſx bool, sx bool) bool { return ſx && sx }
`,
			why:       "long s (U+017F) and s upcase to the same S, so only one of the two may take isSx",
			isChanged: true,
			isSettled: false,
		},
		{
			name: "collidingResultAndParameter",
			src: `package p
func split(ix bool) (ıx bool) { ıx = ix; return }
`,
			why:       "parameters and results share one signature scope, so a result collides with a parameter",
			isChanged: true,
			isSettled: false,
		},
		{
			name: "threeWayCollision",
			src: `package p
func three(ix bool, ıx bool, İx bool) bool { return ix && ıx && İx }
`,
			why:       "İ (U+0130) is already uppercase so İx is exported and unfixable; ix and ıx still contend for isIx",
			isChanged: true,
			isSettled: false,
		},
		{
			name: "underscoreLed",
			src: `package p
func under(_verbose bool) bool { return _verbose }
`,
			why:       "is_verbose has no upper-case boundary after the prefix, so the rule rejects it",
			isChanged: false,
			isSettled: false,
		},
		{
			name: "caselessScript",
			src: `package p
func cjk(有効 bool) bool { return 有効 }
`,
			why:       "有 has no upper case, so no is-prefixed rename of it can satisfy the rule",
			isChanged: false,
			isSettled: false,
		},
		{
			name: "nestedSignatureSharingTheName",
			src: `package p
func shadowed(ready bool) bool {
	inner := func(ready bool) bool { return ready }
	return inner(ready)
}
`,
			why:       "the nested declaration already shadows the enclosing one, so both rename and neither is left reported",
			isChanged: true,
			isSettled: true,
		},
		{
			name: "nestedSignaturesSharingTheNameThreeDeep",
			src: `package p
func deep(ready bool) bool {
	one := func(ready bool) bool {
		two := func(ready bool) bool { return ready }
		return two(ready)
	}
	return one(ready)
}
`,
			why:       "withdrawing on nesting alone strands every level but the outermost, and each stranding is permanent",
			isChanged: true,
			isSettled: true,
		},
		{
			name: "bodylessNestedSignature",
			src: `package p
func fb(g func(sx bool), ſx bool) bool { _ = g; return ſx }
`,
			why:       "the nested func TYPE has no body, so its parameter name has no reference anywhere to be captured",
			isChanged: true,
			isSettled: true,
		},
		{
			name: "nestedSignatureReadingNothingOutside",
			src: `package p
func apart(ix bool) bool {
	inner := func(ıx bool) bool { return ıx }
	return inner(ix)
}
`,
			why:       "distinct names still rename together when the nested body reads nothing from the enclosing signature",
			isChanged: true,
			isSettled: true,
		},
		{
			name: "nestedSignatureCapturingTheEnclosingName",
			src: `package p
func held(ix bool) bool {
	inner := func(ıx bool) bool { return ıx && !ix }
	return inner(ix)
}
`,
			why:       "renaming both would leave `isIx && !isIx`, which builds, vets clean and is constant false, so the nested name is reported and not rewritten",
			isChanged: true,
			isSettled: false,
		},
		{
			name: "plainRename",
			src: `package p
func ok(ready bool) bool { return ready }
`,
			why:       "a name with no collision and no case obstacle must still be rewritten, or the cases above prove nothing",
			isChanged: true,
			isSettled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			once := analyzed(t, tc.src).applied()

			assert.NoError(t, compile(once).err, "%s: the rewritten source must build", tc.why)
			assert.Equal(t, tc.isChanged, once != tc.src, "%s", tc.why)

			settled := analyzed(t, once)
			assert.Equal(t, once, settled.applied(), "a second -fix must change nothing: %s", tc.why)
			assert.Equal(t, tc.isSettled, len(settled.diagnostics) == 0,
				"%s: whatever is still reported here is reported on every run forever", tc.why)
		})
	}
}

// TestEveryProposedNameSatisfiesTheRule names the other half of the promise. A
// fix that proposes a name the analyzer reports again is a rewrite that buys
// nothing and churns the source every run, and repeated runs grow the name one
// "is" at a time.
func TestEveryProposedNameSatisfiesTheRule(t *testing.T) {
	t.Parallel()

	fixable := analyzed(t, `package p
func plain(ready bool) bool { return ready }
func under(_verbose bool) bool { return _verbose }
func cjk(有効 bool) bool { return 有効 }
func doubled(isready bool) bool { return isready }
`)
	proposed := fixable.proposals()
	require.NotEmpty(t, proposed, "a fixture with fixable names must yield proposals")

	for _, name := range proposed {
		assert.True(t, wellNamed(name), "the analyzer proposed %q, which it would report again", name)
	}
}
