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

	"golang.org/x/tools/go/analysis"
)

// Building the rename fix. This half EDITS the user's source: it rewrites every
// reference to the renamed symbol and sweeps the prose that mentions it, so its
// contracts are about scope — which references belong to this symbol, and how
// far a comment sweep may reach.

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
func fixesFor(pass *analysis.Pass, name *ast.Ident, isFixable fixable) []analysis.SuggestedFix {
	if !bool(isFixable) || token.IsExported(name.Name) {
		return nil
	}
	proposed := identName("is" + upperFirst(identName(name.Name)))
	obj := pass.TypesInfo.Defs[name]
	if collides(obj.Parent(), proposed) {
		return nil
	}
	edits := append(
		renameEdits(pass, obj, proposed),
		commentEdits(pass, obj, proposed)...,
	)
	slices.SortFunc(edits, func(a, b analysis.TextEdit) int { return int(a.Pos - b.Pos) })
	return []analysis.SuggestedFix{{
		Message:   fmt.Sprintf("rename %s to %s", name.Name, proposed),
		TextEdits: edits,
	}}
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

// renameEdits rewrites obj's declaration and every reference to proposed.
// Signature names are only referenceable from their own signature and body, so
// the declaring file contains every ident that resolves to obj.
func renameEdits(pass *analysis.Pass, obj types.Object, proposed identName) []analysis.TextEdit {
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
func commentEdits(pass *analysis.Pass, obj types.Object, proposed identName) []analysis.TextEdit {
	file := fileOf(pass, obj.Pos())
	doc, lo, hi := sweepScope(enclosingFunc(file, obj.Pos()))
	groups := groupsWithin(file, lo, hi)
	if doc != nil {
		groups = append(groups, doc)
	}
	var edits []analysis.TextEdit
	for _, group := range groups {
		for _, comment := range group.List {
			edits = append(edits, wordEdits(comment, identName(obj.Name()), proposed)...)
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
func wordEdits(comment *ast.Comment, old, proposed identName) []analysis.TextEdit {
	var edits []analysis.TextEdit
	for from := 0; ; {
		i := strings.Index(comment.Text[from:], string(old))
		if i < 0 {
			return edits
		}
		at := from + i
		from = at + 1
		if isWord(commentText(comment.Text), byteOffset(at), byteCount(len(old))) {
			edits = append(edits, editAt(comment.Pos()+token.Pos(at), old, proposed))
			from = at + len(old)
		}
	}
}

// editAt replaces the len(old) bytes at pos with proposed.
func editAt(pos token.Pos, old, proposed identName) analysis.TextEdit {
	return analysis.TextEdit{Pos: pos, End: pos + token.Pos(len(old)), NewText: []byte(proposed)}
}

// commentText is the raw source text of a comment, comment markers included.
type commentText string

// byteOffset is a byte offset into a comment's source text.
type byteOffset int

// byteCount is a length in bytes of a comment-text match.
type byteCount int

// isWord reports whether the n bytes of text at offset `at` are delimited on
// both sides by non-identifier runes; the start and end of text count as
// boundaries (DecodeRune on an empty string yields RuneError, which is not an
// identifier rune). A mention inside a longer identifier (dryRun, laundry)
// therefore never matches.
func isWord(text commentText, at byteOffset, n byteCount) bool {
	before, _ := utf8.DecodeLastRuneInString(string(text)[:at])
	after, _ := utf8.DecodeRuneInString(string(text)[int(at)+int(n):])
	return !isIdentRune(boundaryRune(before)) && !isIdentRune(boundaryRune(after))
}

// boundaryRune is the rune adjacent to a comment-text match, tested for Go-identifier membership.
type boundaryRune rune

// isIdentRune reports whether r can appear in a Go identifier.
func isIdentRune(r boundaryRune) bool {
	return rune(r) == '_' || unicode.IsLetter(rune(r)) || unicode.IsDigit(rune(r))
}
