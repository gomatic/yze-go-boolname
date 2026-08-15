package boolname

import (
	"go/ast"
	"go/token"
	"go/types"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
)

// Deciding what to rename an ill-named boolean to. Nothing here edits source,
// and nothing here can see the other names the same pass is about to propose —
// that is reconcile.go's job. Every contract in this file is about whether a
// name is free of the code AS IT STANDS: free of the reads a shadow would
// rebind, and free of the rule that would report the proposal all over again.
// Free of the symbols already in scope is NOT the test and never was — Go lets
// a declaration shadow one, and treating that as a collision withholds a fix
// permanently for source that would have been correct.

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
// whose shadowing would capture a read is a collision, which capture.go
// decides. And a proposal the rule itself would report again is no proposal
// at all: a first rune with no upper case — an underscore, or a caseless script
// such as Han — leaves the prefix without the word boundary the rule requires,
// so "is_verbose" and "is有効" are reported the moment they are written and grow
// another "is" on every subsequent run.
func proposalFor(pass *analysis.Pass, name *ast.Ident, obj types.Object, isFixable fixable) identName {
	if !bool(isFixable) || token.IsExported(name.Name) {
		return ""
	}
	proposed := identName("is" + upperFirst(identName(name.Name)))
	if !wellNamed(proposed) || collides(pass, obj, proposed) {
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
