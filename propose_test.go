package boolname

// White-box tests for what the analyzer proposes. Every contract here is about
// a name being FREE: of the scope, of the other names the same pass proposes,
// and of the rule that would report the proposal again.

import (
	"go/ast"
	"go/types"
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

// TestReconciledLetsTheFirstClaimantKeepTheName names reconciled's claim. Two
// candidates proposing one identifier is the defect this pass exists to catch,
// and which of them keeps it is decided by source order rather than by
// traversal luck, or the same input yields different fixes on different runs. A
// nested signature contends too: the analyzer refuses to shadow a name that
// already exists, so it refuses to shadow one it is about to introduce.
func TestReconciledLetsTheFirstClaimantKeepTheName(t *testing.T) {
	t.Parallel()
	src := analyzed(t, `package p
func siblings(ix bool, ıx bool) bool { return ix && ıx }
func nested(ix bool) bool {
	inner := func(ıx bool) bool { return ıx }
	return inner(ix)
}
func elsewhere(ix bool) bool { return ix }
`)
	require.Len(t, src.diagnostics, 5, "every ill-named boolean is reported, fix or no fix")

	for i, tc := range []struct{ want, why string }{
		{want: "rename ix to isIx", why: "the first claimant in a signature keeps the name"},
		{want: "", why: "its sibling must yield, or the signature declares isIx twice"},
		{want: "rename ix to isIx", why: "a different signature is a separate contest"},
		{want: "", why: "a nested signature would shadow the name the enclosing one just took"},
		{want: "rename ix to isIx", why: "an unrelated signature never contends"},
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

// TestOverlapsAnswersTheSameBothWays names overlaps' claim. It decides whether
// two proposals contend, and it is handed the pair in whatever order the
// traversal produced them. A predicate that answered differently for (outer,
// inner) than for (inner, outer) would let a colliding proposal through
// whenever the traversal happened to reach the nested signature first.
func TestOverlapsAnswersTheSameBothWays(t *testing.T) {
	t.Parallel()
	src := checked(t, `package p
func outer(ready bool) bool {
	inner := func(steady bool) bool { return steady }
	return inner(ready)
}
func apart(distinct bool) bool { return distinct }
`)
	scopeOf := func(param string) *types.Scope {
		var declared *types.Scope
		ast.Inspect(src.file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if ok && id.Name == param && src.pass.TypesInfo.Defs[id] != nil {
				declared = src.pass.TypesInfo.Defs[id].Parent()
			}
			return true
		})
		require.NotNil(t, declared, "no declaration of %s", param)
		return declared
	}
	enclosing, nested, elsewhere := scopeOf("ready"), scopeOf("steady"), scopeOf("distinct")

	assert.True(t, overlaps(enclosing, enclosing), "a scope always overlaps itself")
	assert.True(t, overlaps(enclosing, nested), "an enclosing signature contends with a nested one")
	assert.True(t, overlaps(nested, enclosing), "and the nested one contends with it, whichever comes first")
	assert.False(t, overlaps(enclosing, elsewhere), "sibling signatures never contend")
	assert.False(t, overlaps(elsewhere, enclosing), "in either order")
}

// TestEveryFixCompiles names the analyzer's central promise about its own
// output: source it rewrites still builds. The promise is not discharged by
// checking a proposal against the scope as it stands, because the other names
// the same pass proposes are not in that scope yet — two parameters of one
// signature renamed to the same identifier is "redeclared in this block", and
// no golden comparison can see it.
func TestEveryFixCompiles(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		src       string
		why       string
		isChanged bool
	}{
		{
			name: "collidingSiblings",
			src: `package p
func dotless(ix bool, ıx bool) bool { return ix && ıx }
`,
			why:       "ASCII i and dotless ı (U+0131) upcase to the same I, so only one of the two may take isIx",
			isChanged: true,
		},
		{
			name: "collidingSiblingsAcrossAFold",
			src: `package p
func longs(ſx bool, sx bool) bool { return ſx && sx }
`,
			why:       "long s (U+017F) and s upcase to the same S, so only one of the two may take isSx",
			isChanged: true,
		},
		{
			name: "collidingResultAndParameter",
			src: `package p
func split(ix bool) (ıx bool) { ıx = ix; return }
`,
			why:       "parameters and results share one signature scope, so a result collides with a parameter",
			isChanged: true,
		},
		{
			name: "threeWayCollision",
			src: `package p
func three(ix bool, ıx bool, İx bool) bool { return ix && ıx && İx }
`,
			why:       "İ (U+0130) is already uppercase so İx is exported and unfixable; ix and ıx still contend for isIx",
			isChanged: true,
		},
		{
			name: "underscoreLed",
			src: `package p
func under(_verbose bool) bool { return _verbose }
`,
			why:       "is_verbose has no upper-case boundary after the prefix, so the rule rejects it",
			isChanged: false,
		},
		{
			name: "caselessScript",
			src: `package p
func cjk(有効 bool) bool { return 有効 }
`,
			why:       "有 has no upper case, so no is-prefixed rename of it can satisfy the rule",
			isChanged: false,
		},
		{
			name: "plainRename",
			src: `package p
func ok(ready bool) bool { return ready }
`,
			why:       "a name with no collision and no case obstacle must still be rewritten, or the cases above prove nothing",
			isChanged: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			once := analyzed(t, tc.src).applied()

			assert.NoError(t, compile(once).err, "%s: the rewritten source must build", tc.why)
			assert.Equal(t, tc.isChanged, once != tc.src, "%s", tc.why)

			twice := analyzed(t, once).applied()
			assert.Equal(t, once, twice, "a second -fix must change nothing: %s", tc.why)
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
