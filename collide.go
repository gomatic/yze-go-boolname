package boolname

import (
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// The same capture question as capture.go, asked from the other side. Here one
// name is proposed against the source AS IT STANDS, so the symbol it would
// shadow — or be shadowed by — is already declared and already read, and the
// only thing to decide is whether the shadow falls over any of those reads.
// Nothing here may refuse on the identifier merely being taken: Go permits
// shadowing, and every refusal is permanent, because the analyzer reports the
// name again on every run and the source it judges never changes.

// collides reports whether proposed may not be given to obj. Two ways it may
// not, and only two. obj's OWN scope already declares it: that is "redeclared
// in this block", it does not compile, and no fact about reads makes it legal.
// Or giving obj the identifier would shadow a declaration whose reads the
// shadow would rebind — either a symbol above obj that something inside obj's
// scope reads, or a declaration below obj that sits over a read of obj.
//
// Anything else is ordinary shadowing, which Go permits and which moves
// nothing, and refusing it is not caution: the analyzer reports the name on
// every run forever and offers no other fix, so a refusal here is permanent.
// `func f(isReady bool) { g := func(ready bool) bool { return ready } }` is the
// shape that made this worth separating — the identifier is taken, the literal
// reads nothing of the enclosing one, and the rename it was refused builds,
// vets clean and reports nothing.
func collides(pass *analysis.Pass, obj types.Object, proposed identName) bool {
	scope := obj.Parent()
	return scope.Lookup(string(proposed)) != nil ||
		shadowsAReadAbove(pass, scope, proposed) ||
		shadowedByAReadBelow(pass, obj, scope, proposed)
}

// shadowsAReadAbove reports whether proposed names a symbol declared outside
// scope — an enclosing function, the file, the package — that something inside
// scope reads. Declaring proposed in scope shadows that symbol throughout it,
// so exactly those reads change meaning.
//
// The lookup is made AT scope's own position, and that is the whole
// correctness of it. LookupParent handed token.NoPos is position-blind: it
// takes the innermost TEXTUAL declaration of the name, which for a local
// declared BELOW scope is a symbol nothing inside scope can see. Asking whether
// that one is read there answers no, the rename is offered, and it captures the
// read of the outer symbol the identifier actually resolved to — reproduced as
// `var isVerbose = true` read inside a literal with `isVerbose := false` after
// it, which this rewrote to `isVerbose && isVerbose`, and in the type-shaped
// version to source that does not compile at all. Restricting the lookup to
// declarations visible at scope.Pos() skips the later local and reaches the
// object a read there resolves to. Package-scope, file-scope and cross-file
// declarations are unaffected because Go makes them position-independent and
// go/types records them at no position — measured, not assumed.
func shadowsAReadAbove(pass *analysis.Pass, scope *types.Scope, proposed identName) bool {
	_, shadowed := scope.LookupParent(string(proposed), scope.Pos())
	return shadowed != nil && readsWithin(pass, shadowed, scope)
}

// shadowedByAReadBelow reports whether any scope nested below scope declares
// proposed while holding a read of obj. There the shadow runs the other way:
// the nested declaration is already in place, and the rename walks under it, so
// the reads of obj sitting inside that nested scope are the ones it captures.
// The walk is the whole subtree because the declaration may sit at any depth —
// a block, a loop body or a further literal — and a shape one level deeper than
// wherever a bound was put is a shape the bound silently rewrites.
func shadowedByAReadBelow(pass *analysis.Pass, obj types.Object, scope *types.Scope, proposed identName) bool {
	for i := range scope.NumChildren() {
		child := scope.Child(i)
		if (child.Lookup(string(proposed)) != nil && readsWithin(pass, obj, child)) ||
			shadowedByAReadBelow(pass, obj, child, proposed) {
			return true
		}
	}
	return false
}
