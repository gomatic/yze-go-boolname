package boolname

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"unicode"
	"unicode/utf8"
)

// Deciding what to rename an ill-named boolean to. Nothing here edits source;
// every contract is about whether a name is FREE — free of the symbols already
// in scope, free of the other names this same pass proposes, and free of the
// rule that would report the proposal all over again.

// candidate is an ill-named boolean the analyzer will report, paired with the
// rename it offers. An empty proposal is the analyzer saying it sees the
// problem and declines to rewrite it.
type candidate struct {
	name     *ast.Ident
	obj      types.Object
	proposed identName
}

// proposalFor returns the deterministic rename to offer ("is" + upper-cased
// first rune, so unexported-ness is always preserved), or the empty name when
// renaming is not provably safe. Signature names are safe to rename because Go
// makes them referenceable only from their own signature scope and function
// body — never from a _test.go file or another package — and that includes
// bodyless signatures (interface methods, func-type fields and variables),
// whose names have no references at all.
//
// Three things withhold the proposal. An exported-looking name is outside the
// heuristic's lowercase domain and has references this pass cannot see. A name
// already visible in, enclosing, or nested within the signature scope is a
// collision. And a proposal the rule itself would report again is no proposal
// at all: a first rune with no upper case — an underscore, or a caseless script
// such as Han — leaves the prefix without the word boundary the rule requires,
// so "is_verbose" and "is有効" are reported the moment they are written and grow
// another "is" on every subsequent run.
func proposalFor(name *ast.Ident, obj types.Object, isFixable fixable) identName {
	if !bool(isFixable) || token.IsExported(name.Name) {
		return ""
	}
	proposed := identName("is" + upperFirst(identName(name.Name)))
	if !wellNamed(proposed) || collides(obj.Parent(), proposed) {
		return ""
	}
	return proposed
}

// reconciled withdraws every proposal that an earlier candidate's proposal
// would meet, so what the pass emits is collision-free as a SET and not merely
// one fix at a time. collides answers for the scope as it stands, where none of
// these names is declared yet, so it cannot see this: `func f(ix, ıx bool)`
// upper-cases both first runes to I and proposes isIx twice, which is
// "redeclared in this block". The first candidate to want an identifier keeps
// it and the rest are reported without a fix; the withheld ones become ordinary
// collisions on the next run, once the winner is declared.
//
// First means first VISITED, which is the AST traversal's order and not source
// order: an enclosing signature is always reached before one nested inside it,
// so in `func a(g func(ix bool), ıx bool)` the outer signature's ıx wins over
// the ix declared earlier in the line. That is deterministic — the traversal
// and the file order are both fixed — but it is not positional, and reading it
// as positional would make the winner look arbitrary.
func reconciled(candidates []candidate) []candidate {
	kept := make([]candidate, 0, len(candidates))
	for _, reported := range candidates {
		kept = append(kept, reported.against(kept))
	}
	return kept
}

// against returns c with its proposal withdrawn when any earlier candidate
// already proposes the same name where the two would meet.
func (c candidate) against(earlier []candidate) candidate {
	if c.proposed == "" || !slices.ContainsFunc(earlier, c.meets) {
		return c
	}
	c.proposed = ""
	return c
}

// meets reports whether other's proposal would land on c's: the same name in
// the same scope (a redeclaration, which does not compile) or in a scope
// enclosing or enclosed by it (a shadowing, which collides already refuses for
// names that exist).
func (c candidate) meets(other candidate) bool {
	return other.proposed == c.proposed && overlaps(other.obj.Parent(), c.obj.Parent())
}

// overlaps reports whether a name declared in one scope is visible from the
// other: the same scope, or one enclosing the other. Which of the pair is
// passed first is an accident of the order the analyzer visited them in, so the
// answer may not depend on it.
func overlaps(a, b *types.Scope) bool {
	return encloses(a, b) || encloses(b, a)
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

// identName is a Go identifier name under the boolean-naming check, or the predicate-prefixed name proposed to replace one.
type identName string

// upperFirst upcases name's first rune, decoding it (rather than the lead byte)
// so a multi-byte initial such as the é of "état" round-trips correctly.
func upperFirst(name identName) string {
	r, size := utf8.DecodeRuneInString(string(name))
	return string(unicode.ToUpper(r)) + string(name)[size:]
}

// collides reports whether proposed is already declared in the signature scope
// or any scope enclosing it (function-body locals share the signature scope;
// file and package scopes enclose it), or in any scope nested within it, where
// the renamed identifier would be shadowed.
func collides(scope *types.Scope, proposed identName) bool {
	if _, obj := scope.LookupParent(string(proposed), token.NoPos); obj != nil {
		return true
	}
	return declaredWithin(scope, proposed)
}

// declaredWithin reports whether name is declared in any scope nested below scope.
func declaredWithin(scope *types.Scope, name identName) bool {
	for i := range scope.NumChildren() {
		child := scope.Child(i)
		if child.Lookup(string(name)) != nil || declaredWithin(child, name) {
			return true
		}
	}
	return false
}
