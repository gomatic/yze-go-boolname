package boolname

import (
	"go/ast"
	"go/token"
	"go/types"
	"unicode"
	"unicode/utf8"
)

// Deciding what to rename an ill-named boolean to. Nothing here edits source,
// and nothing here can see the other names the same pass is about to propose —
// that is reconcile.go's job. Every contract in this file is about whether a
// name is free of the code AS IT STANDS: free of the symbols already in scope,
// and free of the rule that would report the proposal all over again.

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
