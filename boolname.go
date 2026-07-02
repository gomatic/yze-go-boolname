// Package boolname provides a go/analysis analyzer enforcing the gomatic Go
// boolean naming standard: boolean identifiers carry an is/has/can/should/will
// predicate prefix, or an Enabled/Disabled flag suffix. For parameters and
// named results it offers a mechanical is-prefix rename fix.
package boolname

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var prefixes = []string{"is", "has", "can", "should", "will"}

// message is the diagnostic format; its one verb is the ill-named identifier.
const message = "boolean %s should use an is/has/can/should/will prefix or an Enabled/Disabled suffix"

// Analyzer reports boolean fields, parameters, and results that are not named as
// predicates or flags.
var Analyzer = &analysis.Analyzer{
	Name:     "boolname",
	Doc:      "reports boolean identifiers lacking an is/has/can/should/will prefix or an Enabled/Disabled suffix",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// Registration declares this analyzer to the yze framework.
var Registration = goyze.Registration{
	Name:       "boolname",
	Categories: []goyze.Category{"naming"},
	URL:        "https://docs.gomatic.dev/yze/boolname",
	Analyzer:   Analyzer,
}

// run reports each ill-named boolean field, parameter, and named result. Only
// signature names (parameters and results) are fixable: a struct-field rename
// could break references in _test.go files or other packages, which the yze
// driver does not load.
func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.StructType)(nil), (*ast.FuncType)(nil)}, func(n ast.Node) {
		_, isStruct := n.(*ast.StructType)
		for _, field := range fieldsOf(n) {
			for _, name := range field.Names {
				checkName(pass, name, isFixableParam(!isStruct))
			}
		}
	})
	return nil, nil
}

// fieldsOf returns the fields a node contributes: a struct's fields, or a
// function signature's parameters and results.
func fieldsOf(n ast.Node) []*ast.Field {
	if st, ok := n.(*ast.StructType); ok {
		return st.Fields.List
	}
	ft := n.(*ast.FuncType)
	return append(listOf(ft.Params), listOf(ft.Results)...)
}

func listOf(fields *ast.FieldList) []*ast.Field {
	if fields == nil {
		return nil
	}
	return fields.List
}

// isFixableParam names the isFixable parameter of checkName; rename it to the real domain concept.
type isFixableParam bool

// checkName reports name when it is boolean but not predicate- or flag-named,
// attaching a rename fix when isFixable and the rename is provably safe. The
// blank identifier carries no name to constrain and is skipped.
func checkName(pass *analysis.Pass, name *ast.Ident, isFixable isFixableParam) {
	if name.Name == "_" {
		return
	}
	if !isBoolean(pass, name) || wellNamed(nameParam(name.Name)) {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:            name.Pos(),
		End:            name.End(),
		Message:        fmt.Sprintf(message, name.Name),
		SuggestedFixes: fixesFor(pass, name, isFixable),
	})
}

// fixesFor returns the deterministic rename fix ("is" + upper-cased first rune,
// so unexported-ness is always preserved), or nil when renaming is not provably
// safe. Signature names are safe to rename because Go makes them referenceable
// only from their own signature scope and function body — never from a _test.go
// file or another package — and that includes bodyless signatures (interface
// methods, func-type fields and variables), whose names have no references at
// all. Exported-looking names are outside the heuristic's lowercase domain and
// a proposed name already visible in, enclosing, or nested within the signature
// scope is a collision; both keep the diagnostic fix-free. The fix rewrites the
// code references and sweeps the symbol's scope comments (doc + body) so prose
// mentions of the old name do not go stale.
func fixesFor(pass *analysis.Pass, name *ast.Ident, isFixable isFixableParam) []analysis.SuggestedFix {
	if !bool(isFixable) || token.IsExported(name.Name) {
		return nil
	}
	proposed := "is" + upperFirst(nameParam(name.Name))
	obj := pass.TypesInfo.Defs[name]
	if collides(obj.Parent(), proposedParam(proposed)) {
		return nil
	}
	edits := append(
		renameEdits(pass, obj, proposedParam(proposed)),
		commentEdits(pass, obj, proposedParam(proposed))...,
	)
	slices.SortFunc(edits, func(a, b analysis.TextEdit) int { return int(a.Pos - b.Pos) })
	return []analysis.SuggestedFix{{
		Message:   fmt.Sprintf("rename %s to %s", name.Name, proposed),
		TextEdits: edits,
	}}
}

// nameParam names the name parameter of upperFirst; rename it to the real domain concept.
type nameParam string

// upperFirst upcases name's first rune, decoding it (rather than the lead byte)
// so a multi-byte initial such as the é of "état" round-trips correctly.
func upperFirst(name nameParam) string {
	r, size := utf8.DecodeRuneInString(string(name))
	return string(unicode.ToUpper(r)) + string(name)[size:]
}

// proposedParam names the proposed parameter of collides; rename it to the real domain concept.
type proposedParam string

// collides reports whether proposed is already declared in the signature scope
// or any scope enclosing it (function-body locals share the signature scope;
// file and package scopes enclose it), or in any scope nested within it, where
// the renamed identifier would be shadowed.
func collides(scope *types.Scope, proposed proposedParam) bool {
	if _, obj := scope.LookupParent(string(proposed), token.NoPos); obj != nil {
		return true
	}
	return declaredWithin(scope, string(proposed))
}

// declaredWithin reports whether name is declared in any scope nested below scope.
func declaredWithin(scope *types.Scope, name string) bool {
	for i := range scope.NumChildren() {
		child := scope.Child(i)
		if child.Lookup(name) != nil || declaredWithin(child, name) {
			return true
		}
	}
	return false
}

// renameEdits rewrites obj's declaration and every reference to proposed.
// Signature names are only referenceable from their own signature and body, so
// the declaring file contains every ident that resolves to obj.
func renameEdits(pass *analysis.Pass, obj types.Object, proposed proposedParam) []analysis.TextEdit {
	var edits []analysis.TextEdit
	ast.Inspect(fileOf(pass, obj.Pos()), func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && resolvesTo(pass, id, obj) {
			edits = append(edits, analysis.TextEdit{Pos: id.Pos(), End: id.End(), NewText: []byte(string(proposed))})
		}
		return true
	})
	return edits
}

// commentEdits rewrites word-boundary mentions of obj's old name in the
// comments that belong to the renamed symbol's scope: the enclosing function
// declaration's doc comment and every comment inside its body. For a func
// literal only comments inside the literal's own range are swept — the
// enclosing function's doc describes the outer function, not the literal's
// parameter. A symbol with no enclosing function (an interface method, a
// func-type struct field) owns no prose, so nothing is swept.
func commentEdits(pass *analysis.Pass, obj types.Object, proposed proposedParam) []analysis.TextEdit {
	file := fileOf(pass, obj.Pos())
	doc, lo, hi := sweepScope(enclosingFunc(file, obj.Pos()))
	groups := groupsWithin(file, lo, hi)
	if doc != nil {
		groups = append(groups, doc)
	}
	var edits []analysis.TextEdit
	for _, group := range groups {
		for _, comment := range group.List {
			edits = append(edits, wordEdits(comment, obj.Name(), string(proposed))...)
		}
	}
	return edits
}

// enclosingFunc returns the innermost function declaration or literal whose
// range contains pos, or nil when pos is inside neither (an interface method or
// a func-type struct field). Inspect visits parents before children, so the
// last containing candidate is the innermost.
func enclosingFunc(file *ast.File, pos token.Pos) ast.Node {
	var enclosing ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if isFuncNode(n) && n.Pos() <= pos && pos < n.End() {
			enclosing = n
		}
		return true
	})
	return enclosing
}

// isFuncNode reports whether n declares a function scope that owns comments: a
// function declaration or a function literal.
func isFuncNode(n ast.Node) bool {
	switch n.(type) {
	case *ast.FuncDecl, *ast.FuncLit:
		return true
	}
	return false
}

// sweepScope returns the comment scope a rename sweeps: a function declaration
// contributes its doc comment and its body's range; a func literal contributes
// only its own range (never the enclosing function's doc). A nil node
// contributes nothing.
func sweepScope(n ast.Node) (*ast.CommentGroup, token.Pos, token.Pos) {
	switch fn := n.(type) {
	case *ast.FuncDecl:
		lo, hi := blockSpan(fn.Body)
		return fn.Doc, lo, hi
	case *ast.FuncLit:
		return nil, fn.Pos(), fn.End()
	}
	return nil, token.NoPos, token.NoPos
}

// blockSpan returns body's range, or an empty range for a bodyless declaration
// (a forward declaration implemented outside Go), which no comment can be
// inside of.
func blockSpan(body *ast.BlockStmt) (token.Pos, token.Pos) {
	if body == nil {
		return token.NoPos, token.NoPos
	}
	return body.Pos(), body.End()
}

// groupsWithin returns the file's comment groups positioned inside [lo, hi].
func groupsWithin(file *ast.File, lo, hi token.Pos) []*ast.CommentGroup {
	var groups []*ast.CommentGroup
	for _, group := range file.Comments {
		if lo <= group.Pos() && group.End() <= hi {
			groups = append(groups, group)
		}
	}
	return groups
}

// wordEdits returns one edit per word-boundary occurrence of old in comment's
// text. A comment's text is contiguous source bytes, so a byte offset into it
// maps directly onto the fset via the comment's position.
func wordEdits(comment *ast.Comment, old, proposed string) []analysis.TextEdit {
	var edits []analysis.TextEdit
	for from := 0; ; {
		i := strings.Index(comment.Text[from:], old)
		if i < 0 {
			return edits
		}
		at := from + i
		from = at + 1
		if isWord(textParam(comment.Text), at, len(old)) {
			edits = append(edits, editAt(comment.Pos()+token.Pos(at), old, proposed))
			from = at + len(old)
		}
	}
}

// editAt replaces the len(old) bytes at pos with proposed.
func editAt(pos token.Pos, old, proposed string) analysis.TextEdit {
	return analysis.TextEdit{Pos: pos, End: pos + token.Pos(len(old)), NewText: []byte(proposed)}
}

// textParam names the text parameter of isWord; rename it to the real domain concept.
type textParam string

// isWord reports whether the n bytes of text at offset `at` are delimited on
// both sides by non-identifier runes; the start and end of text count as
// boundaries (DecodeRune on an empty string yields RuneError, which is not an
// identifier rune). A mention inside a longer identifier (dryRun, laundry)
// therefore never matches.
func isWord(text textParam, at, n int) bool {
	before, _ := utf8.DecodeLastRuneInString(string(text)[:at])
	after, _ := utf8.DecodeRuneInString(string(text)[at+n:])
	return !isIdentRune(rParam(before)) && !isIdentRune(rParam(after))
}

// rParam names the r parameter of isIdentRune; rename it to the real domain concept.
type rParam rune

// isIdentRune reports whether r can appear in a Go identifier.
func isIdentRune(r rParam) bool {
	return rune(r) == '_' || unicode.IsLetter(rune(r)) || unicode.IsDigit(rune(r))
}

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

// isBoolean reports whether name's defined object has a boolean underlying type.
// name is a non-blank field, parameter, or result identifier, which always has a
// defined object.
func isBoolean(pass *analysis.Pass, name *ast.Ident) bool {
	basic, ok := pass.TypesInfo.Defs[name].Type().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func wellNamed(name nameParam) bool {
	return hasPredicatePrefix(nameParam(string(name))) || hasFlagSuffix(nameParam(string(name)))
}

func hasPredicatePrefix(name nameParam) bool {
	for _, prefix := range prefixes {
		if matchesPrefix(string(name), prefix) {
			return true
		}
	}
	return false
}

// matchesPrefix reports whether name begins with prefix at a word boundary.
func matchesPrefix(name, prefix string) bool {
	if !strings.HasPrefix(strings.ToLower(name), prefix) {
		return false
	}
	rest := name[len(prefix):]
	return rest != "" && startsUpper(restParam(rest))
}

// restParam names the rest parameter of startsUpper; rename it to the real domain concept.
type restParam string

// startsUpper reports whether rest begins with an uppercase or titlecase rune,
// marking the word boundary that follows a predicate prefix. Decoding the first
// rune (rather than the lead byte) admits non-ASCII boundaries such as "État".
func startsUpper(rest restParam) bool {
	r, _ := utf8.DecodeRuneInString(string(rest))
	return unicode.IsUpper(r) || unicode.IsTitle(r)
}

func hasFlagSuffix(name nameParam) bool {
	return strings.HasSuffix(string(name), "Enabled") || strings.HasSuffix(string(name), "Disabled")
}
