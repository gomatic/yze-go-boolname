// Package boolname provides a go/analysis analyzer enforcing the gomatic Go
// boolean naming standard: boolean identifiers carry an is/has/can/should/will
// predicate prefix, or an Enabled/Disabled flag suffix.
package boolname

import (
	"go/ast"
	"go/types"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var prefixes = []string{"is", "has", "can", "should", "will"}

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

// run reports each ill-named boolean field, parameter, and named result.
func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.StructType)(nil), (*ast.FuncType)(nil)}, func(n ast.Node) {
		for _, field := range fieldsOf(n) {
			for _, name := range field.Names {
				checkName(pass, name)
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

// checkName reports name when it is boolean but not predicate- or flag-named.
// The blank identifier carries no name to constrain and is skipped.
func checkName(pass *analysis.Pass, name *ast.Ident) {
	if name.Name == "_" {
		return
	}
	if isBoolean(pass, name) && !wellNamed(name.Name) {
		pass.Reportf(name.Pos(), "boolean %s should use an is/has/can/should/will prefix or an Enabled/Disabled suffix", name.Name)
	}
}

// isBoolean reports whether name's defined object has a boolean underlying type.
// name is a non-blank field, parameter, or result identifier, which always has a
// defined object.
func isBoolean(pass *analysis.Pass, name *ast.Ident) bool {
	basic, ok := pass.TypesInfo.Defs[name].Type().Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}

func wellNamed(name string) bool {
	return hasPredicatePrefix(name) || hasFlagSuffix(name)
}

func hasPredicatePrefix(name string) bool {
	for _, prefix := range prefixes {
		if matchesPrefix(name, prefix) {
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
	return rest != "" && isUpper(rest[0])
}

func hasFlagSuffix(name string) bool {
	return strings.HasSuffix(name, "Enabled") || strings.HasSuffix(name, "Disabled")
}

func isUpper(b byte) bool {
	return b >= 'A' && b <= 'Z'
}
