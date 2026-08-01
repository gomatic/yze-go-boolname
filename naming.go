package boolname

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
)

// Deciding whether a name is already well-formed. Every rule here is ASCII-only
// on purpose: the recognised prefixes are pure ASCII, so a Unicode case mapping
// that changes byte length can never misalign a slice taken after a match.

// resolvesTo reports whether id declares or references obj.
func resolvesTo(pass *analysis.Pass, id *ast.Ident, obj types.Object) bool {
	return pass.TypesInfo.Defs[id] == obj || pass.TypesInfo.Uses[id] == obj
}

// fileOf returns the file containing pos. Every reported ident comes from a
// file in pass.Files, so the lookup always succeeds.
func fileOf(pass *analysis.Pass, pos token.Pos) *ast.File {
	return pass.Files[slices.IndexFunc(pass.Files, func(file *ast.File) bool {
		return file.FileStart <= pos && pos < file.FileEnd
	})]
}

// isBoolean reports whether name's defined object has a boolean underlying
// type, unwrapping ONE pointer level so a *bool name is checked like a bool.
// Deeper indirection (**bool) and generic ~bool type parameters are
// deliberately out of scope, like the vars and consts the analyzer never
// visits. name is a non-blank field, parameter, or result identifier, which
// always has a defined object.
func isBoolean(pass *analysis.Pass, name *ast.Ident) bool {
	typ := pass.TypesInfo.Defs[name].Type().Underlying()
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem().Underlying()
	}
	basic, ok := typ.(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func wellNamed(name identName) bool {
	return hasPredicatePrefix(name) || hasFlagSuffix(name)
}

func hasPredicatePrefix(name identName) bool {
	for _, prefix := range prefixes {
		if matchesPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// matchesPrefix reports whether name begins with prefix at a word boundary. The
// recognized prefixes are pure ASCII, so the match is ASCII-only: a non-ASCII
// leading rune (İ, U+0130) never matches a prefix, and name[len(prefix):] is
// always an exact rune-boundary slice — a Unicode lowercase mapping that changes
// byte length can never misalign it.
func matchesPrefix(name identName, prefix predicatePrefix) bool {
	if !hasASCIIPrefixFold(name, prefix) {
		return false
	}
	rest := name[len(prefix):]
	return rest != "" && startsUpper(nameRest(rest))
}

// hasASCIIPrefixFold reports whether name begins with the pure-ASCII lowercase
// prefix under ASCII-only case folding: each name byte matches its prefix byte
// exactly or with the 0x20 case bit toggled, so no byte outside ASCII (and no
// multi-byte case mapping) can ever match.
func hasASCIIPrefixFold(name identName, prefix predicatePrefix) bool {
	if len(name) < len(prefix) {
		return false
	}
	for i := range len(prefix) {
		if name[i]|0x20 != prefix[i] {
			return false
		}
	}
	return true
}

// nameRest is the remainder of an identifier name after a candidate predicate prefix.
type nameRest string

// startsUpper reports whether rest begins with an uppercase or titlecase rune,
// marking the word boundary that follows a predicate prefix. Decoding the first
// rune (rather than the lead byte) admits non-ASCII boundaries such as "État".
func startsUpper(rest nameRest) bool {
	r, _ := utf8.DecodeRuneInString(string(rest))
	return unicode.IsUpper(r) || unicode.IsTitle(r)
}

func hasFlagSuffix(name identName) bool {
	return strings.HasSuffix(string(name), "Enabled") || strings.HasSuffix(string(name), "Disabled")
}
