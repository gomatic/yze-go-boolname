package boolname

import (
	"go/ast"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"
)

// Deciding whether giving one identifier to two symbols would change what
// something READS. Every question here is about a pair of scopes and the
// references between them, and none of them is answerable by the type checker:
// a capture BUILDS, which is the whole difficulty. reconcile.go asks it of two
// names the same pass is about to introduce.

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

// encloses reports whether outer is inner itself or an ancestor of it. The walk
// runs the whole scope-ancestry chain because the distance between two
// contending signatures is unbounded: a bare block or an enclosing literal puts
// the nested one two scopes down, and a statement that opens a scope for its own
// header puts it three.
func encloses(outer, inner *types.Scope) bool {
	for scope := inner; scope != nil; scope = scope.Parent() {
		if scope == outer {
			return true
		}
	}
	return false
}
