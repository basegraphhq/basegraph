package enumvalidator

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "enumvalidator",
	Doc:  "checks that enum fields only use defined constants, not string literals",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Define enum types to check
	enumTypes := map[string]bool{
		"Provider":       true,
		"Capability":     true,
		"CredentialType": true,
	}

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}

			// Check each assignment
			for i, lhs := range assign.Lhs {
				if i >= len(assign.Rhs) {
					continue
				}

				// Check if left side is an enum field
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					if isEnumField(pass, sel, enumTypes) {
						// Check if right side is a string literal
						if isStringLiteral(assign.Rhs[i]) {
							pass.Reportf(assign.Pos(),
								"enum field %s assigned string literal; use defined constant instead",
								sel.Sel.Name)
						}
					}
				}
			}

			return true
		})
	}
	return nil, nil
}

func isEnumField(pass *analysis.Pass, sel *ast.SelectorExpr, enumTypes map[string]bool) bool {
	// Get type info for the selector
	if t := pass.TypesInfo.TypeOf(sel); t != nil {
		if named, ok := t.(*types.Named); ok {
			return enumTypes[named.Obj().Name()]
		}
	}
	return false
}

func isStringLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}
