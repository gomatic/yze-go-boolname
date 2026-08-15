package boolname

// White-box tests for what makes two symbols unable to share one identifier.
// Every contract here is about a PAIR of scopes and the references between
// them: whether shadowing would rebind a read, and whether the answer depends
// on anything but that read.

import (
	"fmt"
	"go/ast"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
