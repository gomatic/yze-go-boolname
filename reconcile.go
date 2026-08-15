package boolname

import (
	"slices"

	"golang.org/x/tools/go/analysis"
)

// Reconciling the proposals against EACH OTHER. Everything in propose.go asks
// whether a name is free of the code as it stands; nothing there can see the
// other names this same pass is about to introduce. This is where that is
// settled — which of two candidates wanting one identifier keeps it. WHETHER
// two may simply both have it is capture.go's, because it is the same question
// propose.go asks of a name already in the source.

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
// would land on c's. It asks contends ONE way round, and that is safe only
// because contends answers the same both ways — which is a coupling to that
// promise rather than an assumption about this pair, since the order the two
// arrive in is the traversal's accident.
func (c candidate) meets(pass *analysis.Pass) func(candidate) bool {
	return func(other candidate) bool {
		return other.proposed == c.proposed && contends(pass, other, c)
	}
}
