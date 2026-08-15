package boolname

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Deciding whether giving one identifier to two symbols would change what
// something READS. Every question here is about a pair of scopes and the
// references between them, and none of them is answerable by the type checker:
// a capture BUILDS, which is the whole difficulty. reconcile.go asks it of two
// names the same pass is about to introduce; propose.go asks it of one name
// against a symbol the source already declares. It is ONE question, and the two
// callers got different answers to it for as long as they each had their own.

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
// none does.
//
// Two shapes make the rename exact by construction, and both were stranded
// while this asked only whether the scopes nest. Identical original names:
// inner already shadows outer, so nothing inside inner's scope reads outer's
// object and there is nothing to capture. A bodyless nested signature — an
// interface method, a func-type field or parameter — has no body for a read to
// sit in at all.
func captures(pass *analysis.Pass, outer, inner candidate) bool {
	nested := inner.obj.Parent()
	return encloses(outer.obj.Parent(), nested) && readsWithin(pass, outer.obj, nested)
}

// encloses reports whether outer is inner itself or an ancestor of it. The walk
// runs the whole scope-ancestry chain because the distance between two
// contending signatures is unbounded: a bare block or an enclosing literal puts
// the nested one two scopes down, and a statement that opens a scope for its own
// header puts it three.
//
// Its includes-self case is unreachable today and no case can be written for
// it, which is worth saying rather than papering over: contends answers equal
// scopes with its own first disjunct before captures is ever called, so every
// pair arriving here has outer strictly enclosing inner or not enclosing it at
// all. Starting the walk at inner.Parent() would answer identically. That is a
// COUPLING and not decoration — removing or reordering contends' same-scope
// disjunct makes the case live, and this is the guard that would then have to
// hold it.
func encloses(outer, inner *types.Scope) bool {
	for scope := inner; scope != nil; scope = scope.Parent() {
		if scope == outer {
			return true
		}
	}
	return false
}

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
// so exactly those reads change meaning. LookupParent takes the INNERMOST such
// declaration, which is the one a read there resolves to today.
func shadowsAReadAbove(pass *analysis.Pass, scope *types.Scope, proposed identName) bool {
	_, shadowed := scope.LookupParent(string(proposed), token.NoPos)
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

// readsWithin reports whether any identifier READING obj sits inside scope,
// which is the one question every shadowing decision above reduces to: a
// declaration shadows a name throughout its own scope, so it rebinds exactly
// the reads sitting there and nothing else.
//
// READS, never declarations — and the reason is worth stating precisely,
// because the obvious reason is wrong. It is NOT that counting declarations
// would make every pair contend: the only ident this would add is obj's own
// declaration, and today that declaration always sits in a scope enclosing the
// region being searched, so no caller would get a different answer. It is that
// a declaration is what the shadow is measured AGAINST rather than something
// the shadow moves. The narrowing is inert exactly as long as no caller hands
// this a region containing obj's own declaration — which contends prevents by
// answering equal scopes with its own first disjunct, and collides by
// answering obj's own scope with Lookup, both before reaching here. Remove or
// reorder either and this filter starts deciding answers.
//
// The scope's extent is the search region, and go/types opens a function scope
// at its signature and widens it to the whole declaration or literal, so the
// region is always a superset of where the name it holds is visible. A read the
// shadow would not in fact reach can therefore be counted; one it would reach
// never escapes. The error this can make is a refusal to rename, never a silent
// rebinding.
func readsWithin(pass *analysis.Pass, obj types.Object, scope *types.Scope) bool {
	var found bool
	ast.Inspect(fileOf(pass, scope.Pos()), func(n ast.Node) bool {
		id, isIdent := n.(*ast.Ident)
		found = found || (isIdent && pass.TypesInfo.Uses[id] == obj && scope.Contains(id.Pos()))
		return !found
	})
	return found
}
