// Package boolname provides a go/analysis analyzer enforcing the gomatic Go
// boolean naming standard on struct fields and function parameters and results:
// bool and *bool names there carry an is/has/can/should/will predicate prefix,
// or an Enabled/Disabled flag suffix. Package-level and local variables,
// constants, and generic (~bool-constrained) type parameters are deliberately
// out of scope. For parameters and named results it offers a mechanical
// is-prefix rename fix.
package boolname

import (
	"fmt"
	"go/ast"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// predicatePrefix is one of the sanctioned boolean predicate prefixes.
type predicatePrefix string

var prefixes = []predicatePrefix{"is", "has", "can", "should", "will"}

// message is the diagnostic format; its one verb is the ill-named identifier.
const message = "boolean %s should use an is/has/can/should/will prefix or an Enabled/Disabled suffix"

// Analyzer reports boolean fields, parameters, and results that are not named as
// predicates or flags.
var Analyzer = &analysis.Analyzer{
	Name:     "boolname",
	Doc:      "reports boolean struct fields, parameters, and results lacking an is/has/can/should/will prefix or an Enabled/Disabled suffix",
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
//
// Every candidate is collected before any is reported, because a rename is only
// safe against the source as the whole pass will leave it. A proposal checked
// against the scope as it stands is blind to the OTHER names this pass proposes
// — they are not declared yet — and two signature names renamed to one
// identifier is "redeclared in this block".
func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	var found []candidate
	insp.Preorder([]ast.Node{(*ast.StructType)(nil), (*ast.FuncType)(nil)}, func(n ast.Node) {
		_, isStruct := n.(*ast.StructType)
		for _, field := range fieldsOf(n) {
			found = append(found, candidatesIn(pass, field, fixable(!isStruct))...)
		}
	})
	for _, reported := range reconciled(pass, found) {
		pass.Report(reported.diagnostic(pass))
	}
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

// fixable is whether a rename fix may be offered for an ill-named boolean: signature names are fixable, struct fields are not.
type fixable bool

// candidatesIn returns the ill-named booleans among one field's names.
func candidatesIn(pass *analysis.Pass, field *ast.Field, isFixable fixable) []candidate {
	var found []candidate
	for _, name := range field.Names {
		if reported, ok := candidateFor(pass, name, isFixable); ok {
			found = append(found, reported)
		}
	}
	return found
}

// candidateFor returns the candidate for name when it is boolean but not
// predicate- or flag-named. The blank identifier carries no name to constrain
// and is skipped.
func candidateFor(pass *analysis.Pass, name *ast.Ident, isFixable fixable) (candidate, bool) {
	if name.Name == "_" {
		return candidate{}, false
	}
	if !isBoolean(pass, name) || wellNamed(identName(name.Name)) {
		return candidate{}, false
	}
	obj := pass.TypesInfo.Defs[name]
	return candidate{name: name, obj: obj, proposed: proposalFor(name, obj, isFixable)}, true
}

// diagnostic is the report for an ill-named boolean, carrying the rename fix
// when a proposal survived every collision check and none otherwise.
func (c candidate) diagnostic(pass *analysis.Pass) analysis.Diagnostic {
	return analysis.Diagnostic{
		Pos:            c.name.Pos(),
		End:            c.name.End(),
		Message:        fmt.Sprintf(message, c.name.Name),
		SuggestedFixes: fixesFor(pass, c),
	}
}
