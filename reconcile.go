package boolname

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
)

// Reconciling the proposals against EACH OTHER. Everything in propose.go asks
// whether a name is free of the code as it stands; nothing there can see the
// other names this same pass is about to introduce. This is where that is
// settled — which of two candidates wanting one identifier keeps it, and when
// two may simply both have it.

// reconciled withdraws every proposal that an earlier candidate's proposal
// would meet, so what the pass emits is collision-free as a SET and not merely
// one fix at a time. collides answers for the scope as it stands, where none of
// these names is declared yet, so it cannot see this: `func f(ix, ıx bool)`
// upper-cases both first runes to I and proposes isIx twice, which is
// "redeclared in this block". The first candidate to want an identifier keeps
// it and the rest are reported without a fix; the withheld ones become ordinary
// collisions on the next run, once the winner is declared.
//
// A NESTED signature is not such a case by itself, and withdrawing there
// without asking is permanent damage rather than caution. Two scopes, one
// inside the other, may hold the same identifier — that is shadowing, and it
// compiles — so the rename is only wrong where it changes what something
// resolves to, which contends decides by looking. A nested name withdrawn
// wrongly is stranded FOREVER: no later run renames it either, because by then
// the enclosing name is declared and collides refuses to shadow it.
//
// First means first VISITED, which is the AST traversal's order and not source
// order: an enclosing signature is always reached before one nested inside it,
// so in `func a(g func(ix bool), ıx bool)` the outer signature's ıx is the
// earlier candidate although the ix is declared earlier in the line. That is
// deterministic — the traversal and the file order are both fixed — but it is
// not positional, and reading it as positional would make the winner look
// arbitrary.
func reconciled(pass *analysis.Pass, candidates []candidate) []candidate {
	kept := make([]candidate, 0, len(candidates))
	for _, reported := range candidates {
		kept = append(kept, reported.against(pass, kept))
	}
	return kept
}

// against returns c with its proposal withdrawn when any earlier candidate
// already proposes the same name where the two would meet.
func (c candidate) against(pass *analysis.Pass, earlier []candidate) candidate {
	if c.proposed == "" || !slices.ContainsFunc(earlier, c.meets(pass)) {
		return c
	}
	c.proposed = ""
	return c
}

// meets returns the predicate deciding whether an earlier candidate's proposal
// would land on c's.
func (c candidate) meets(pass *analysis.Pass) func(candidate) bool {
	return func(other candidate) bool {
		return other.proposed == c.proposed && contends(pass, other, c)
	}
}

// contends reports whether two candidates proposing one identifier may not both
// take it. In ONE scope they may not, ever: that is "redeclared in this block",
// and it does not compile whether or not either name is ever used. In NESTED
// scopes they may, unless renaming both would capture — which captures decides.
// Which of the pair is passed first is an accident of the order the analyzer
// visited them in, so the answer may not depend on it.
func contends(pass *analysis.Pass, a, b candidate) bool {
	return a.obj.Parent() == b.obj.Parent() || captures(pass, a, b) || captures(pass, b, a)
}

// captures reports whether giving outer and inner one identifier would change
// what something READS. The rename rewrites every identifier resolving to
// outer's object, and inner's declaration shadows that identifier throughout
// inner's scope — so the rename is wrong exactly when one of those rewritten
// identifiers reads outer's object from INSIDE inner's scope, and exact when
// none does. Reads only: outer's own declaration is what does the shadowing,
// not something shadowed, and counting it would make every pair contend.
//
// Two shapes make the rename exact by construction, and both were stranded
// while this asked only whether the scopes nest. Identical original names:
// inner already shadows outer, so nothing inside inner's scope reads outer's
// object and there is nothing to capture. A bodyless nested signature — an
// interface method, a func-type field or parameter — has no body for a read to
// sit in at all.
func captures(pass *analysis.Pass, outer, inner candidate) bool {
	nested := inner.obj.Parent()
	if !encloses(outer.obj.Parent(), nested) {
		return false
	}
	return slices.ContainsFunc(referencesTo(pass, outer.obj), func(id *ast.Ident) bool {
		return pass.TypesInfo.Uses[id] == outer.obj && nested.Contains(id.Pos())
	})
}

// encloses reports whether outer is inner itself or an ancestor of it.
func encloses(outer, inner *types.Scope) bool {
	for scope := inner; scope != nil; scope = scope.Parent() {
		if scope == outer {
			return true
		}
	}
	return false
}
