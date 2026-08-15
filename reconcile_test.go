package boolname

// White-box tests for reconciling the proposals against each other: which of
// two candidates wanting one identifier keeps it, and what the rewrite must
// never do to earn that answer.

import (
	"fmt"
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

// TestCaptureIsSeenAtEveryScopeDistance names the dimension every other case in
// this package holds constant. All of them put the nested signature one scope
// below the enclosing one, so nothing here could tell encloses' walk up the
// whole scope chain from a walk that looked only at the immediate parent —
// which is a live rewrite of the constant-false bug this file exists to
// prevent, and one the gate passes at full coverage. Each placement below sits
// the nested signature a different distance down: a bare block and a func
// literal are two scopes away, a statement that opens a scope for its own
// header is three, and a composite literal element is one, the same as no
// placement at all.
//
// Both directions run at every distance, because the distance is not itself the
// danger: the capturing variant must leave the nested name reported forever,
// and the variant that reads nothing outside must rename both and settle.
func TestCaptureIsSeenAtEveryScopeDistance(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ name, src string }{
		{name: "noPlacement", src: `package p
func f(ix bool) bool {
	inner := func(ıx bool) bool { return ıx%s }
	return inner(ix)
}
`},
		{name: "compositeLiteralElement", src: `package p
func f(ix bool) bool {
	fns := []func(bool) bool{
		func(ıx bool) bool { return ıx%s },
	}
	return fns[0](ix)
}
`},
		{name: "bareBlock", src: `package p
func f(ix bool) bool {
	{
		inner := func(ıx bool) bool { return ıx%s }
		return inner(ix)
	}
}
`},
		{name: "deferredLiteral", src: `package p
func f(ix bool) bool {
	defer func() {
		inner := func(ıx bool) bool { return ıx%s }
		_ = inner(ix)
	}()
	return ix
}
`},
		{name: "selectCase", src: `package p
func f(ix bool) bool {
	ch := make(chan bool, 1)
	ch <- ix
	select {
	case <-ch:
		inner := func(ıx bool) bool { return ıx%s }
		return inner(ix)
	}
}
`},
		{name: "threeSignaturesDeep", src: `package p
func f(ix bool) bool {
	mid := func(kx bool) bool {
		deep := func(ıx bool) bool { return ıx%s }
		return deep(kx)
	}
	return mid(ix)
}
`},
		{name: "ifBody", src: `package p
func f(ix bool) bool {
	if ix {
		inner := func(ıx bool) bool { return ıx%s }
		return inner(ix)
	}
	return ix
}
`},
		{name: "forBody", src: `package p
func f(ix bool) bool {
	for ix {
		inner := func(ıx bool) bool { return ıx%s }
		return inner(ix)
	}
	return ix
}
`},
		{name: "labelledForBody", src: `package p
func f(ix bool) bool {
loop:
	for ix {
		inner := func(ıx bool) bool { return ıx%s }
		if inner(ix) {
			break loop
		}
	}
	return ix
}
`},
		{name: "switchCase", src: `package p
func f(ix bool) bool {
	switch {
	case ix:
		inner := func(ıx bool) bool { return ıx%s }
		return inner(ix)
	}
	return ix
}
`},
		{name: "typeSwitchCase", src: `package p
func f(ix bool, v any) bool {
	switch v.(type) {
	case int:
		inner := func(ıx bool) bool { return ıx%s }
		return inner(ix)
	}
	return ix
}
`},
	} {
		for _, read := range []struct {
			name       string
			suffix     string
			why        string
			isCaptured bool
		}{
			{name: "capturing", suffix: " && !ix", isCaptured: true, why: "the nested body reads the enclosing ix, so one identifier for both would rebind that read however far apart the two scopes sit"},
			{name: "free", suffix: "", isCaptured: false, why: "the nested body reads nothing from the enclosing signature, so the distance between the scopes is not on its own a reason to strand the name"},
		} {
			t.Run(tc.name+"-"+read.name, func(t *testing.T) {
				t.Parallel()
				src := fmt.Sprintf(tc.src, read.suffix)
				once := analyzed(t, src).applied()

				assert.NoError(t, compile(once).err, "%s", read.why)
				assert.Equal(t, resolution(t, src), resolution(t, once), "%s", read.why)
				assert.Equal(t, read.isCaptured, len(analyzed(t, once).diagnostics) != 0,
					"whatever is still reported here is reported on every run forever: %s", read.why)
			})
		}
	}
}
