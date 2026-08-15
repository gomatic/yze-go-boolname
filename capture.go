package boolname

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Deciding whether giving one identifier to two symbols would change what
// something READS. Every question here is about a pair of scopes and the
// references between them, and none of them is answerable by the type checker:
// a capture BUILDS, which is the whole difficulty. readsWithin is that question
// and it is asked twice: here, of two names the same pass is about to
// introduce, and in collide.go, of one name against a symbol the source already
// declares. It is ONE question, and the two callers got different answers to it
// for as long as they each had their own.

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
